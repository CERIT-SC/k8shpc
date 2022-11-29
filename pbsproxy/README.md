# PBS Proxy

This directory contains docker image that is used for Pod replacement of moved Job from K8s to PBS and also for SSH proxy. SSH proxy container is used to export PVC from K8s to PBS via sshfs. Sshfs is expected to be installed in PBS.

## Customization

### Docker Image

The docker image can be customized using ARG section of the Dockerfile, e.g.:

```
ARG USER_NAME=funnelworker
ARG USER_GROUP=meta
ARG USER_UID=1000
ARG USER_GID=1000
ARG KUBERNETES_VERSION=1.22.0
ARG SSH_PORT=2222
```

* `USER_NAME` is user name that is used in PBS and it is the same user name that will be used in docker image. The docker image should use the same name as PBS.
* `USER_GROUP` is user group in similar manner as the user name.
* `USER_UID` and `USER_GID` is numerical user/group ID that replacement Pod and SSH proxy will run as.
* `KUBERNETES_VERSION` is version of downloaded `kubectl`. It should be at most one version different of running K8s.
* `SSH_PORT` port to be used of SSH proxy (internal port, external should be always 22).

### Deployment

Deployment customisation is done via `config.ini` file, e.g.:
```
[GENPROXY]
metallb_addresspool = privmuni
externaldomain      = dyn.cloud.e-infra.cz
sshport             = 2222
loadbalancerport    = 22
container           = cerit.io/cerit/geant-proxy:v0.9
```

* `metallb_adresspool` is optional address pool name in case of using `metallb` Load Balancer.
* `externaldomain` is optional, it adds annotation for External DNS component so that DNS name is created for Load Balancer service.
* `sshport` is port for SSH proxy, this **must match value from the Docker Image**.
* `loadbalancerport` is required parameter for Load Balancer service to listen at, it should have value `22`.
* `container` name of created Docker Image, it has to be the name of built docker image from this sources.

### PBS Settings

PBS settings are at the very beginning of `run-qsub-gpu.sh.tmpl` and `run-qsub.sh.tmpl` files:
```
#PBS -o zuphux.cerit-sc.cz:logs/$$.stdout
#PBS -e zuphux.cerit-sc.cz:logs/$$.stderr
#PBS -l select=1:ncpus=$CPUL:mem=$MEML:ngpus=$GPUR:scratch_ssd=20gb
```

* `-o zuphux.cerit-sc.cz:logs/$$.stdout` sets where to store standard output of the PBS job. Can be arbitrary location, useful for debugs.
* `-e zuphux.cerit-sc.cz:logs/$$.stderr` sets where to store standard error output of the PBS job.
* `-l select=1:ncpus=$CPUL:mem=$MEML:ngpus=$GPUR:scratch_ssd=20gb` is template for resource allocation, it can contain queue definition and other OpenPBS options as published. Variables `$CPUL`, `$MEML` and `$GPUR` will be replaced at runtime and are not meant for direct customisation.

The file `run-qsub-gpu.sh.tmpl` is used in case of GPU request, `run-qsub.sh.tmpl` is used otherwise.

## Notes

Currently, only `Load Balancer` service type is supported, not `Node Port`.
