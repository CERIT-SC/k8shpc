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

if [ -z $CPUL ]; then
   CPUL=$CPUR
fi

if echo $CPUL | grep -q "m"; then
   CPUL=`echo $CPUL | sed -e 's/m//'`
   CPUL=$[CPUL/1000+1]
fi

if [ -z $MEML ]; then
   MEML=$MEMR
fi

MEML="$[MEML/1073741824+1]gb" # to GB

unset COMMAND; for i in `seq 0 20`; do t=CMD_$i; [[ ! -z ${!t} ]] && COMMAND=(${COMMAND[*]} "'${!t}'"); done

CMD=${COMMAND[*]}

envsubst '$CMD $CONTAINER $ssh_host $ssh_key $MEML $CPUL' < /srv/run-qsub.sh > /tmp/run-qsub.sh

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

exit $exitc
