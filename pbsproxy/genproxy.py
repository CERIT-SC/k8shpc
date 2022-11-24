#!/usr/bin/python3

import os
import sys
import hashlib
import configparser

config = configparser.ConfigParser()
config.read('/srv/config.ini')

if not config.has_section('GENPROXY'):
    print("/srv/config.ini does not have section GENPROXY. Exit.")
    exit(1)

config = config['GENPROXY']

addresspool=config.get('metallb_addresspool', '')
externaldomain=config.get('externaldomain', '')
sshport=config.get('sshport')
lbport=config.get('loadbalancerport')
container=config.get('container')

if not sshport or not lbport:
    print("sshport and loadbalancerport must be defined in GENPROXY section in /srv/config.ini. Exit.")
    exit(1)

def gen_service(name, finalizer):
  global externaldomain
  global addresspool
  svc='''apiVersion: v1
kind: Service
metadata:
  name: {exppodname}
  annotations: {externaldomain}{net}
  finalizers:{finalizer}
spec:
  type: LoadBalancer
  ports:
  - port: {lbport}
    targetPort: {sshport}
  selector:
    app: {exppodname}
  externalTrafficPolicy: Local
'''
  if finalizer != '':
    finalizer = "\n  - "+finalizer
  if externaldomain:
    externaldomain = "\n    external-dns.alpha.kubernetes.io/hostname: "+name+"."+externaldomain
  if addresspool != '':
    addresspool = "\n    metallb.universe.tf/address-pool: "+addresspool
  return svc.format(exppodname=name, net=addresspool, finalizer=finalizer, lbport=lbport, sshport=sshport, externaldomain=externaldomain)

def gen_deployment(name, finalizer, volmounts, volumes):
  global container
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
        image: {container}
        imagePullPolicy: IfNotPresent
        env:
        - name: NAMESPACE
          value: {namespace}
        resources:
          limits:
            cpu: 1
            memory: 256Mi
        ports:
        - containerPort: {sshport}
'''
  if finalizer != '':
    finalizer = "\n  - "+finalizer
  deployment = deployment.format(exppodname=name, finalizer=finalizer, namespace=os.environ.get("NAMESPACE"), sshport=sshport, container=container)
  if volmounts and volumes:
        deployment = deployment+"        volumeMounts:\n"+volmounts+"      volumes:\n"+volumes
  return deployment


volumes=''
volmounts=''
name=[]
mnts=[]
pvcs={}

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
      if pvcs.get(pvc):
        volmounts=volmounts+"        - name: vol-%d\n          mountPath: '%s'\n"%(pvcs.get(pvc), mnt)
      else:
        volmounts=volmounts+"        - name: vol-%d\n          mountPath: '%s'\n"%(i, mnt)
        volumes=volumes+"      - name: vol-%d\n        persistentVolumeClaim:\n          claimName: '%s'\n"%(i,pvc)
        pvcs[pvc]=i
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
    print(gen_service(name, finalizer), end='')
    print("---")
    print(gen_deployment(name, finalizer, volmounts, volumes))
  if sys.argv[i] == "--genmnt":
    for j in mnts:
      print(j+" ", end='')
  if sys.argv[i] == "--genname":
    print(name)
