package clone

import (
	"fmt"
	"time"

	k8scorev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	clonebase "kubevirt.io/api/clone"
	clone "kubevirt.io/api/clone/v1beta1"
	virtv1 "kubevirt.io/api/core/v1"
	snapshotv1 "kubevirt.io/api/snapshot/v1beta1"
	"kubevirt.io/client-go/kubecli"
	"kubevirt.io/client-go/log"

	"kubevirt.io/kubevirt/pkg/storage/snapshot"
)

type Event string

const (
	defaultVerbosityLevel = 2
	unknownTypeErrFmt     = "clone controller expected object of type %s but found object of unknown type"

	SnapshotCreated       Event = "SnapshotCreated"
	SnapshotReady         Event = "SnapshotReady"
	RestoreCreated        Event = "RestoreCreated"
	RestoreCreationFailed Event = "RestoreCreationFailed"
	RestoreReady          Event = "RestoreReady"
	TargetVMCreated       Event = "TargetVMCreated"
	PVCBound              Event = "PVCBound"

	SnapshotDeleted    Event = "SnapshotDeleted"
	SourceDoesNotExist Event = "SourceDoesNotExist"
)

type VMCloneController struct {
	client               kubecli.KubevirtClient
	vmCloneIndexer       cache.Indexer
	snapshotStore        cache.Store
	restoreStore         cache.Store
	vmStore              cache.Store
	snapshotContentStore cache.Store
	pvcStore             cache.Store
	recorder             record.EventRecorder

	vmCloneQueue workqueue.TypedRateLimitingInterface[string]
	hasSynced    func() bool
}

func NewVmCloneController(client kubecli.KubevirtClient, vmCloneInformer, snapshotInformer, restoreInformer, vmInformer, snapshotContentInformer, pvcInformer cache.SharedIndexInformer, recorder record.EventRecorder) (*VMCloneController, error) {
	ctrl := VMCloneController{
		client:               client,
		vmCloneIndexer:       vmCloneInformer.GetIndexer(),
		snapshotStore:        snapshotInformer.GetStore(),
		restoreStore:         restoreInformer.GetStore(),
		vmStore:              vmInformer.GetStore(),
		snapshotContentStore: snapshotContentInformer.GetStore(),
		pvcStore:             pvcInformer.GetStore(),
		recorder:             recorder,
		vmCloneQueue: workqueue.NewTypedRateLimitingQueueWithConfig[string](
			workqueue.DefaultTypedControllerRateLimiter[string](),
			workqueue.TypedRateLimitingQueueConfig[string]{Name: "virt-controller-vmclone"},
		),
	}

	ctrl.hasSynced = func() bool {
		return vmCloneInformer.HasSynced() && snapshotInformer.HasSynced() && restoreInformer.HasSynced() &&
			vmInformer.HasSynced() && snapshotInformer.HasSynced() && pvcInformer.HasSynced()
	}

	_, err := vmCloneInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    ctrl.handleVMClone,
			UpdateFunc: func(oldObj, newObj interface{}) { ctrl.handleVMClone(newObj) },
			DeleteFunc: ctrl.handleVMClone,
		},
	)

	if err != nil {
		return nil, err
	}

	_, err = snapshotInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    ctrl.handleSnapshot,
			UpdateFunc: func(oldObj, newObj interface{}) { ctrl.handleSnapshot(newObj) },
			DeleteFunc: ctrl.handleSnapshot,
		},
	)

	if err != nil {
		return nil, err
	}

	_, err = restoreInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    ctrl.handleRestore,
			UpdateFunc: func(oldObj, newObj interface{}) { ctrl.handleRestore(newObj) },
			DeleteFunc: ctrl.handleRestore,
		},
	)

	if err != nil {
		return nil, err
	}

	_, err = pvcInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    ctrl.handlePVC,
			UpdateFunc: func(oldObj, newObj interface{}) { ctrl.handlePVC(newObj) },
			DeleteFunc: ctrl.handlePVC,
		},
	)

	if err != nil {
		return nil, err
	}

	_, err = vmInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			DeleteFunc: ctrl.handleDeleteVM,
		},
	)

	if err != nil {
		return nil, err
	}
	return &ctrl, nil
}

func (ctrl *VMCloneController) handleVMClone(obj interface{}) {
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}

	vmClone, ok := obj.(*clone.VirtualMachineClone)
	if !ok {
		log.Log.Errorf(unknownTypeErrFmt, clonebase.ResourceVMCloneSingular)
		return
	}

	objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(vmClone)
	if err != nil {
		log.Log.Errorf("vm clone controller failed to get key from object: %v, %v", err, vmClone)
		return
	}

	log.Log.V(defaultVerbosityLevel).Infof("enqueued %q for sync", objName)
	ctrl.vmCloneQueue.Add(objName)
}

func (ctrl *VMCloneController) handleSnapshot(obj interface{}) {
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}

	snapshot, ok := obj.(*snapshotv1.VirtualMachineSnapshot)
	if !ok {
		log.Log.Errorf(unknownTypeErrFmt, "virtualmachinesnapshot")
		return
	}

	if ownedByClone, key := isOwnedByClone(snapshot); ownedByClone {
		ctrl.vmCloneQueue.AddRateLimited(key)
	}

	snapshotKey, err := cache.MetaNamespaceKeyFunc(snapshot)
	if err != nil {
		log.Log.Object(snapshot).Reason(err).Error("cannot get snapshot key")
		return
	}

	snapshotSourceKeys, err := ctrl.vmCloneIndexer.IndexKeys("snapshotSource", snapshotKey)
	if err != nil {
		log.Log.Object(snapshot).Reason(err).Error("cannot get clone snapshotSourceKeys from snapshotSource indexer")
		return
	}

	snapshotWaitingKeys, err := ctrl.vmCloneIndexer.IndexKeys(string(clone.SnapshotInProgress), snapshotKey)
	if err != nil {
		log.Log.Object(snapshot).Reason(err).Error("cannot get clone snapshotWaitingKeys from " + string(clone.SnapshotInProgress) + " indexer")
		return
	}

	for _, key := range append(snapshotSourceKeys, snapshotWaitingKeys...) {
		ctrl.vmCloneQueue.AddRateLimited(key)
	}
}

func (ctrl *VMCloneController) handleRestore(obj interface{}) {
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}

	restore, ok := obj.(*snapshotv1.VirtualMachineRestore)
	if !ok {
		log.Log.Errorf(unknownTypeErrFmt, "virtualmachinerestore")
		return
	}

	if ownedByClone, key := isOwnedByClone(restore); ownedByClone {
		ctrl.vmCloneQueue.AddRateLimited(key)
	}

	restoreKey, err := cache.MetaNamespaceKeyFunc(restore)
	if err != nil {
		log.Log.Object(restore).Reason(err).Error("cannot get snapshot key")
		return
	}

	restoreWaitingKeys, err := ctrl.vmCloneIndexer.IndexKeys(string(clone.RestoreInProgress), restoreKey)
	if err != nil {
		log.Log.Object(restore).Reason(err).Error("cannot get clone restoreWaitingKeys from " + string(clone.RestoreInProgress) + " indexer")
		return
	}

	for _, key := range restoreWaitingKeys {
		ctrl.vmCloneQueue.AddRateLimited(key)
	}
}

func (ctrl *VMCloneController) handlePVC(obj interface{}) {
	if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok && unknown.Obj != nil {
		obj = unknown.Obj
	}

	pvc, ok := obj.(*k8scorev1.PersistentVolumeClaim)
	if !ok {
		log.Log.Errorf(unknownTypeErrFmt, "persistentvolumeclaim")
		return
	}

	var (
		restoreName string
		exists      bool
	)

	if restoreName, exists = pvc.Annotations[snapshot.RestoreNameAnnotation]; !exists {
		return
	}

	if pvc.Status.Phase != k8scorev1.ClaimBound {
		return
	}

	restoreKey := getKey(restoreName, pvc.Namespace)

	succeededWaitingKeys, err := ctrl.vmCloneIndexer.IndexKeys(string(clone.Succeeded), restoreKey)
	if err != nil {
		log.Log.Object(pvc).Reason(err).Error("cannot get clone succeededWaitingKeys from " + string(clone.Succeeded) + " indexer")
		return
	}

	for _, key := range succeededWaitingKeys {
		ctrl.vmCloneQueue.AddRateLimited(key)
	}
}

func (ctrl *VMCloneController) handleDeleteVM(obj interface{}) {
	vm, ok := obj.(*virtv1.VirtualMachine)
	// When a delete is dropped, the relist will notice a vm in the store not
	// in the list, leading to the insertion of a tombstone object which contains
	// the deleted key/value. Note that this value might be stale.
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Log.Reason(fmt.Errorf("couldn't get object from tombstone %+v", obj)).Error("Failed to process delete notification")
			return
		}
		vm, ok = tombstone.Obj.(*virtv1.VirtualMachine)
		if !ok {
			log.Log.Reason(fmt.Errorf("tombstone contained object that is not a vm %#v", obj)).Error("Failed to process delete notification")
			return
		}
	}
	vmCloneList, err := ctrl.listVmCloneMatchingVM(vm.Namespace, vm.Name)
	if err != nil {
		log.Log.Errorf("error retrieving vm clone list: %v", err.Error())
		return
	}
	log.Log.V(4).Object(vm).Infof("vm clone lis: %v", vmCloneList)
	for _, vmClone := range vmCloneList {
		log.Log.V(4).Object(vm).Infof("vm deleted for vm clone %s", vmClone.Name)
		objName, err := cache.DeletionHandlingMetaNamespaceKeyFunc(vmClone)
		if err != nil {
			log.Log.Errorf("vm clone controller failed to get key from object: %v, %v", err, vmClone)
			return
		}
		ctrl.vmCloneQueue.AddRateLimited(objName)
	}

	return
}

func (ctrl *VMCloneController) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer ctrl.vmCloneQueue.ShutDown()

	log.Log.Info("Starting clone controller")
	defer log.Log.Info("Shutting down clone controller")

	if !cache.WaitForCacheSync(
		stopCh,
		ctrl.hasSynced,
	) {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(ctrl.runWorker, time.Second, stopCh)
	}

	<-stopCh
	return nil
}

func (ctrl *VMCloneController) Execute() bool {
	key, quit := ctrl.vmCloneQueue.Get()
	if quit {
		return false
	}
	defer ctrl.vmCloneQueue.Done(key)

	err := ctrl.execute(key)

	if err != nil {
		log.Log.Reason(err).Infof("reenqueuing clone %v", key)
		ctrl.vmCloneQueue.AddRateLimited(key)
	} else {
		log.Log.V(defaultVerbosityLevel).Infof("processed clone %v", key)
		ctrl.vmCloneQueue.Forget(key)
	}
	return true
}

func (ctrl *VMCloneController) runWorker() {
	for ctrl.Execute() {
	}
}

// takes a namespace and returns all vm clone with the specified target vm name
func (ctrl *VMCloneController) listVmCloneMatchingVM(namespace, name string) ([]*clone.VirtualMachineClone, error) {
	return ctrl.filterVmClone(namespace, func(vmClone *clone.VirtualMachineClone) bool {
		return vmClone.Spec.Target.Name == name
	})
}

func (ctrl *VMCloneController) filterVmClone(namespace string, filter func(*clone.VirtualMachineClone) bool) ([]*clone.VirtualMachineClone, error) {
	objs, err := ctrl.vmCloneIndexer.ByIndex(cache.NamespaceIndex, namespace)
	if err != nil {
		return nil, err
	}

	var vmClones []*clone.VirtualMachineClone
	for _, obj := range objs {
		vmClone := obj.(*clone.VirtualMachineClone)

		if filter(vmClone) {
			vmClones = append(vmClones, vmClone)
		}
	}
	return vmClones, nil
}
