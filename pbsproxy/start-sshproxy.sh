#!/bin/bash

if kubectl get secret -n ${NAMESPACE} sshkey &> /dev/null; then
  export ssh_key=`kubectl get secret -n ${NAMESPACE} sshkey -o jsonpath='{.data.ssh-privatekey}' | base64 -d`
else 
  echo -e "\n" | ssh-keygen -q -N ''
  kubectl create secret generic sshkey --from-file=ssh-privatekey=/home/funnelworker/.ssh/id_rsa --from-file=ssh-publickey=/home/funnelworker/.ssh/id_rsa.pub -n ${NAMESPACE}
  export ssh_key=`cat /home/funnelworker/.ssh/id_rsa`
fi

kubectl get secret -n ${NAMESPACE} sshkey -o jsonpath='{.data.ssh-publickey}' | base64 -d > /home/funnelworker/.ssh/authorized_keys && chmod 0600 /home/funnelworker/.ssh/authorized_keys 

set | grep 'PROXY\|KUBERNETES' | sed -e 's/^/export /' >> /home/funnelworker/.bashrc; 
touch /tmp/sshd.log
/usr/sbin/sshd -E /tmp/sshd.log
tail -f /tmp/sshd.log
