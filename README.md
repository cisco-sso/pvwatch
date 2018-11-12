PersistentVolume Watchdog
=

Kubernetes operator waiting on events that relate to common and known issues, automatically resolves conditions described in upstream trackers below:

* https://github.com/kubernetes/cloud-provider-openstack/issues/150
* https://github.com/kubernetes/kubernetes/issues/33128

Exposes standard prometheus metrics with one application specific `pvwatch`:

|label | Description|
|------|------------|
|event | The observed event|
|msg   | Information about the action taken, `ok` means the watchdog determined it should act, otherwise describes why it didn't|
|pod   | Pod that has failed to attach the PersistentVolume |
|node  | Kubernetes node that scheduled the pod|
|err   | Any error the watchdog encoutered during event processing|
