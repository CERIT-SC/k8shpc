#!/bin/bash
#PBS -o zuphux.cerit-sc.cz:logs/${PBS_JOBID}.stdout
#PBS -e zuphux.cerit-sc.cz:logs/${PBS_JOBID}.stderr
#PBS -l select=1:ncpus=$CPUL:mem=$MEML:ngpus=$GPUR:scratch_ssd=20gb:vnode=^samson
#PBS -q gpu

sshkey="$ssh_key"

echo "$sshkey" > /storage/brno12-cerit/home/funnelworker/id_rsa.$$
chmod 0600 /storage/brno12-cerit/home/funnelworker/id_rsa.$$

image=$CONTAINER

mounts="$MNT"

cache='/storage/brno12-cerit/home/funnelworker/cache'
sif=`echo $image | sed -e 's/\//-/g'`

mkdir $cache 2> /dev/null

cd $SCRATCHDIR || exit 1

export TMPDIR=$SCRATCHDIR


while ! mkdir "$cache/$sif.lck" 2> /dev/null; do
        sleep 1;
done

if ! [ -f "$cache/$sif" ]; then
        singularity pull "$cache/$sif" "docker://$image"
fi

rmdir "$cache/$sif.lck"

while ! host ${ssh_host} &> /dev/null; do
 sleep 5;
 echo "Waiting for host ${ssh_host} to be available..."
done

j=1;
for i in $mounts; do
  mkdir "$j"
  k=1;
  while true; do
     sshfs -o IdentityFile=/storage/brno12-cerit/home/funnelworker/id_rsa.$$,UserKnownHostsFile=/dev/null,StrictHostKeyChecking=no funnelworker@${ssh_host}:"$i" "$j" && break;
     echo "Waiting for ssh endpoint to be ready"
     sleep $k;
     k=$[k*2];
     if [ $k -gt 2048 ]; then
             echo "Timeout waiting for ssh endpoint";
             exit 2;
     fi
  done
  binds=(${binds[*]} '--bind' "$j:$i")
  j=$[j+1]
done

$ENVS

singularity run ${binds[*]} -i "$cache/$sif" $CMD

ret=$?

j=$[j-1]

for i in `seq 1 $j`; do
        umount "$i" && rmdir "$i";
done

rm -f /storage/brno12-cerit/home/funnelworker/id_rsa.$$

exit $ret
