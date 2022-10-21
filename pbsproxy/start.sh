#!/bin/bash

echo -e "\n" | ssh-keygen -q -N ''

cp /home/funnelworker/.ssh/id_rsa.pub /home/funnelworker/.ssh/authorized_keys
chmod 0600 /home/funnelworker/.ssh/authorized_keys

set | grep 'PROXY\|KUBERNETES' | sed -e 's/^/export /' >> /home/funnelworker/.bashrc; 
/usr/sbin/sshd

if echo ${POD_NAME} | grep -q -e '-.....$'; then
    export POD_NAME=`echo ${POD_NAME} | sed -e 's/-.....$//'`
fi

envsubst < /srv/service.yaml > /tmp/service.yaml

/usr/bin/kubectl create -f /tmp/service.yaml -n ${NAMESPACE}

export ssh_key=`cat /home/funnelworker/.ssh/id_rsa`

export ssh_host=$POD_NAME.dyn.cloud.e-infra.cz

envsubst '$COMMAND $CONTAINER $ssh_host $ssh_key' < /srv/run-qsub.sh > /tmp/run-qsub.sh

jobid=`/usr/bin/qsub /tmp/run-qsub.sh`

while true; do
    state=`qstat -x -f $jobid | grep job_state | sed -e 's/.*= //'`
    if [ $state == 'F' -o $state = 'E' ]; then
	 exitc=`qstat -x -f $jobid | grep Exit_status | sed -e 's/.*= //'`
	 break;
    fi
    sleep 5;
done

/usr/bin/kubectl delete -f /tmp/service.yaml -n ${NAMESPACE}

echo $exitc

exit $exitc
