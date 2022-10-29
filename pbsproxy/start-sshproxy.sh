#!/bin/bash

kubectl get secret -n ${NAMESPACE} sshkey -o jsonpath='{.data.ssh-publickey}' | base64 -d > /home/funnelworker/.ssh/authorized_keys || echo "Failed to get ssh key."

chmod 0600 /home/funnelworker/.ssh/authorized_keys 

set | grep 'PROXY\|KUBERNETES' | sed -e 's/^/export /' >> /home/funnelworker/.bashrc; 
touch /tmp/sshd.log
/usr/sbin/sshd -E /tmp/sshd.log
tail -f /tmp/sshd.log
