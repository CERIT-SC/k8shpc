#!/bin/bash
#PBS -o zuphux.cerit-sc.cz:logs/${PBS_JOBID}.stdout
#PBS -e zuphux.cerit-sc.cz:logs/${PBS_JOBID}.stderr
#PBS -l select=1:scratch_ssd=20gb

sshkey="$ssh_key"

echo "$sshkey" > /storage/brno12-cerit/home/funnelworker/id_rsa.$$
chmod 0600 /storage/brno12-cerit/home/funnelworker/id_rsa.$$

if test -z $CONTAINER; then
    image='nfcore/sarek:2.7.2'
else
    image=$CONTAINER
fi
mounts=('/mnt')
cmd=('/bin/bash' '-ue' $COMMAND)

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
  sshfs -o IdentityFile=/storage/brno12-cerit/home/funnelworker/id_rsa.$$,UserKnownHostsFile=/dev/null,StrictHostKeyChecking=no funnelworker@${ssh_host}:"$i" "$j" || exit 2
  binds=(${binds[*]} '--bind' "$j:$i")
  j=$[j+1]
done

singularity run ${binds[*]} -i "$cache/$sif" ${cmd[*]}

ret=$?

j=$[j-1]

for i in `seq 1 $j`; do
        umount "$i" && rmdir "$i";
done

rm -f /storage/brno12-cerit/home/funnelworker/id_rsa.$$

exit $ret
