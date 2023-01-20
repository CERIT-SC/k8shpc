# Admission Controller
This repository contains the code and deployment files for mutating admission webhook 
that transparently mutates the Kubernetes Job's YAML. We expect that you have a Kubernetes cluster up and running and that
you are allowed to deploy webhooks (usually requires to be a cluster admin).

## Deployment
Every admission controller needs for proper functionality:
1. TLS certificate
2. Service object which makes the controller reachable
3. Deployment with controller's code
4. Webhook configuration

### Namespace
In order to deploy the webhook, firstly make sure you created namespace `k8shpc-ns`. It is possible to use any other namespace but then
remember to change the `namespace` variable in all YAMLs and in `commands.sh`.

### Context
Change the context in the `commands.sh` to yours. If you do not know what context is yours, you can use command
`kubectl config get-contexts` and search for line with asterisk at the beginning. 

### TLS certificate
Generate the TLS certificate as shown in `certificate/commands.sh`. The script makes use of Kubernetes built-in signer `kubelet-serving`.
**DO NOT** forget to generate client key and client certificate with `openssl` command (as shown in comment in the script) in the `certificate` directory before running the script. Otherwise, the script will fail with an
error message approximately "File client.csr not found". If everything went well, a Secret named "k8shpc-mutating-webhook-cert-secret" should be present in the `k8shpc-ns`
namespace or in the namespace of your choice. We advise checking, if the secret contains right data with commands:

1. `kubectl get secrets k8shpc-mutating-webhook-cert-secret -n [namespace] -o json | jq -r '.data["tls.key"]' | base64 -D`
   Should start with "-----BEGIN RSA PRIVATE KEY-----"
2. `kubectl get secrets k8shpc-mutating-webhook-cert-secret -n [namespace] -o json | jq -r '.data["tls.crt"]' | base64 -D`
   Should start with "-----BEGIN CERTIFICATE REQUEST-----"
3. `kubectl get secrets k8shpc-mutating-webhook-cert-secret -n [namespace] -o json | jq -r '.data["ca.crt"]' | base64 -D`
   Should start with "-----BEGIN CERTIFICATE-----"

If the data does not correspond (especially ca.crt), run `kubectl get csr k8shpc-mutating-webhook-svc.[namespace]  -o jsonpath='{.status.certificate}' | base64 -D`. If the output
starts with "-----BEGIN CERTIFICATE-----", manually copy the output to the secret.

### Kubernetes Objects
Deploy YAMLs from directory `deployment` into your Kubernetes cluster. Deploy permissions (rbac.yaml) for ServiceAccounts. If you changed the namespace, it is necessary
to change lines **5, 29, 59** to this namespace. Then deploy Service (service.yaml), if you changed namespace, change line **5** to this namespace. Then deploy the Deployment (deployment.yaml),
if you changed namespace, change line **5** to this namespace. Lastly deploy webhook configuration (mwhkConfig.yaml), if you changed namespace, change lines **6 (before slash), 15** to this namespace.

You can use command `kubectl create -f [filename]` to create the objects.

If everything went right, you should see `k8shpc-mutating-webhook.cerit.io ` in the output of command `kubectl get mutatingwebhookconfigurations.admissionregistration.k8s.io`.

## How to Use
It is enough to add label with key `hpctransfer` and value one of `can, must, cooperative` to the Job, which is eligible to be transferred to the HPC environment.

If this label is present, the admission controller will decide what happens with the Job according to the value. *Must* suggests that Job is moved to HPC environment at all times.
*Can* suggests that the Job can be transferred to the HPC cluster but not necessarily. If Kubernetes Job’s resource requests are smaller than maximum available resources on one node in Kubernetes,
Job stays in Kubernetes cluster. Otherwise, it is moved to the HPC cluster. In the *cooperative* mode the Job is transferred to the HPC cluster only if Kubernetes Job’s resource requests are bigger than currently available free resources on one node in the Kubernetes cluster. 
If there are enough free resources at the moment for Job execution in Kubernetes cluster, the Job remains in the Kubernetes cluster. 

This logic is coded inside `main.go` file and can be extended, removed or arbitrarily changed. 

## Effects
If Job is to be moved to HPC cluster, its image is changed to different one created by administrator who knows the HPC system well.
This image that includes logic necessary to submit the job to HPC cluster, mainly submission and authentication commands as well as several helper scripts that enable 
exporting NFS volumes from Kubernetes Job. We have created image and a set of script that enable submission into PBS world. 

For more information on PBS image and its scripts, see README in https://github.com/CERIT-SC/k8shpc/tree/main/pbsproxy 


