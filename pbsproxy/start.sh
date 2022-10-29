#!/bin/bash

if ! kubectl get secret -n ${NAMESPACE} sshkey &> /dev/null; then
  echo -e "\n" | ssh-keygen -q -N ''
  kubectl create secret generic sshkey --from-file=ssh-privatekey=/home/funnelworker/.ssh/id_rsa --from-file=ssh-publickey=/home/funnelworker/.ssh/id_rsa.pub -n ${NAMESPACE}
fi

export ssh_key=`kubectl get secret -n ${NAMESPACE} sshkey -o jsonpath='{.data.ssh-privatekey}' | base64 -d`

if [ -z "$ssh_key" ]; then
   echo "Failed to get SSH private key from sshkey secret. Exiting."
   exit 1;
fi

if echo ${POD_NAME} | grep -q -e '-.....$'; then
    export POD_NAME=`echo ${POD_NAME} | sed -e 's/-.....$//'`
fi

pvcvar=${!PVC_*}

export pvc=`echo $pvcvar | sed -e 's/^PVC_//' -e 's/_/-/g'`

export mnt=${!pvcvar}

export exppodname="export-$pvc"

envsubst '$exppodname $mnt $pvc' < /srv/ssh-proxy.yaml | kubectl create -f - -n ${NAMESPACE} && sleep 10

export ssh_host=${exppodname}.dyn.cloud.e-infra.cz

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

exit $exitc
