#!/usr/bin/python

import os
import sys
import hashlib

addresspool=os.environ.get('ADDRESS_POOL', 'privmuni')

def gen_service(name, finalizer):
  svc='''apiVersion: v1
kind: Service
metadata:
  name: {exppodname}
  annotations:
    external-dns.alpha.kubernetes.io/hostname: {exppodname}.dyn.cloud.e-infra.cz
    metallb.universe.tf/address-pool: {net}
  finalizers:{finalizer}
spec:
  type: LoadBalancer
  ports:
  - port: 22
    targetPort: 2222
  selector:
    app: {exppodname}
  externalTrafficPolicy: Local
'''
  if finalizer != '':
    finalizer = "\n  - "+finalizer
  return svc.format(exppodname=name, net=addresspool, finalizer=finalizer)

def gen_deployment(name, finalizer, volmounts, volumes):
  deployment='''apiVersion: apps/v1
kind: Deployment
metadata:
  name: {exppodname}
  finalizers:{finalizer}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: {exppodname}
  template:
    metadata:
      labels:
        app: {exppodname}
    spec:
      containers:
      - name: ssh-proxy
        command:
        - /srv/start-sshproxy.sh
        image: cerit.io/cerit/geant-proxy:v0.6
        imagePullPolicy: IfNotPresent
        securityContext:
          runAsUser: 1000
          runAsGroup: 1000
        resources:
          limits:
            cpu: 1
            memory: 256Mi
        ports:
        - containerPort: 2222
'''
  if finalizer != '':
    finalizer = "\n  - "+finalizer
  deployment = deployment.format(exppodname=name, finalizer=finalizer)
  if volmounts and volumes:
        deployment = deployment+"        volumeMounts:\n"+volmounts+"      volumes:\n"+volumes
  return deployment


volumes=''
volmounts=''
name=[]
mnts=[]

i=1
for e in os.environ.keys():
   if e.startswith("PVC_"):
      mnt=os.environ.get(e)
      j=e.find("_", 4)
      if j != -1:
        pvc=e[j+1:]
      else:
        print("Error, wrong PVC defined:"+e)
        exit(1)
      pvc=pvc.replace("_","-")
      name.append(pvc+mnt)
      mnts.append(mnt)
      volmounts=volmounts+"        - name: vol-%d\n          mountPath: '%s'\n"%(i, mnt)
      volumes=volumes+"      - name: vol-%d\n        persistentVolumeClaim:\n          claimName: '%s'\n"%(i,pvc)
      i=i+1

name="".join(sorted(set(name)))

name=hashlib.md5(name.encode('utf-8')).hexdigest()

name="sshexp-"+name
finalizer=''

for i in range(1,len(sys.argv)):
  if sys.argv[i] == "--finalizer":
    finalizer = sys.argv[i+1]
    i = i + 1
  if sys.argv[i] == "--genproxy":
    print(gen_service(name, finalizer))
    print("---")
    print(gen_deployment(name, finalizer, volmounts, volumes))
  if sys.argv[i] == "--genmnt":
    for j in mnts:
      print(j+" "),
  if sys.argv[i] == "--genname":
    print(name)
