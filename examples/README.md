
Install nextflow.

Necessary to create secret in namespace where all transfer jobs will run. 

If run with Kerberos:
```
kubectl create secret generic krbconf --from-file=krb5.keytab --from-file=krb5.conf -n [namespace]
```
Create configuration for PBS scheduler:
```
kubectl create configmap pbsconf --from-file=krb5.keytab --from-file=pbs.conf -n [namespace]
```

Run command:

```
nextflow/launch.sh -C nextflowres.config kuberun hello -head-image 'cerit.io/nextflow/nextflow:22.06.1' -head-memory 4096Mi -head-cpus 1
```
