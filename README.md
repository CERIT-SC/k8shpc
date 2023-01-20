# K8sHPC connector

The goal of this project is to implement a transparent way of submitting jobs from the container platform Kubernetes into HPC environment. There is a number of motivations for such work, one of them being a shift to containerization in modern research and at the same time, dependence on large resources for some computation types or parts of complex workflows. 

There are two straightforward solutions – add more resources to the clusters (to support bigger and more demanding computations) or implement better scheduling mechanisms (to ensure effective available resource division and access). The first solution solves insufficient cluster resources but it is not possible to regularly purchase hardware. Moreover, huge resource wasting can be expected with such setup. The second solution offers a solution on a software level which creates an opportunity for sophisticated implementations and complex logic. Also, implementing a novel scheduling system requires years of dedication, constant maintenance, and improvement and over time, it grows into unmanageable complexity. 

With this project, we try to deliver an implementation that transparently moves the execution of Kubernetes Job to an external system that contains complex scheduler. Implementation is agnostic of the used scheduling system and acts as an optional additional component in container Kubernetes clusters. The solution employs existing Kubernetes concepts and patterns (Admission Controllers), does not require change of the cluster’s default settings and generally, is not dependent on any third-party component. The implementation is easily extendable to support alternative scheduling systems.

On a higher level, the admission controller mutates the Kubernetes Job object with another image that encapsulates software needed for submission into the external system and a managing script. The implementation incorporates a solution for sharing the data between the Kubernetes cluster and the external system both utilizing independent NFS servers as the primary storage solution. To our knowledge, no direct solution is available so we formed a working example that does not require data transport from one system to another. This connector takes authentication into account but leaves proper implementation to the new image assigned to the Kubernetes Job. 

### PoC Implementation
This implementation supports PBSPro scheduling system and Kerberos authentication. The solution was successfully tested with Nextflow [`nf-sarek`](https://github.com/nf-core/sarek/) pipeline and real-life data.

If you would like to use any other scheduling system, the support must be implemented in similar way as for PBS. Some script from `pbsproxy` directory can be 
reused or utilized for inspiration.

## Contents
This repository consists of two main parts:
- Admission Controller Implementation https://github.com/CERIT-SC/k8shpc/tree/main/k8shpc-ac
- Proxy scheduler implementation using PBSPro  https://github.com/CERIT-SC/k8shpc/tree/main/pbsproxy

See READMEs of both directories for more information on individual parts.

## Contact
If interested in more information or contributing, reach us at k8s@ics.muni.cz

-------------------------------------------------------
This project was supported by GÉANT Innovation Programme, Agreement number: SER-22-107_60. For more details: https://community.geant.org/community-programme-portfolio/innovation-programme/funded-projects/

Computational resources were supplied by the project "e-Infrastruktura CZ" (e-INFRA CZ LM2018140 ) supported by the Ministry of Education, Youth and Sports of the Czech Republic.
