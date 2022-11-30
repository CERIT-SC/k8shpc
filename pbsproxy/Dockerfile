FROM debian:11

### Parametrize as needed
# user name, should be the same as user in PBS
ARG USER_NAME=funnelworker 
# user group, should be the sam as group in PBS
ARG USER_GROUP=meta
ARG USER_UID=1000
ARG USER_GID=1000
ARG KUBERNETES_VERSION=1.22.0
# set value above 1024 if sshd will run without root
ARG SSH_PORT=2222

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive  apt-get -y --no-install-recommends install  \
       ed gettext-base procps openssh-server ca-certificates \
       vim bind9-host wget tzdata locales libgssapi3-heimdal netcat-openbsd \
       libedit2 libpython3.9 libtcl8.6 libtk8.6  \
       python3 python python3-pip krb5-user openssh-client && \ 
       wget http://repo.cerit-sc.cz/debian/pool/main/libpbspro_19.0.0-28%2Bdeb11_amd64.deb && dpkg -i libpbspro_19.0.0-28+deb11_amd64.deb && \
       wget http://repo.cerit-sc.cz/debian/pool/main/pbspro-client_19.0.0-28%2Bdeb11_amd64.deb && dpkg -i pbspro-client_19.0.0-28+deb11_amd64.deb && \
       rm -f *.deb && apt-get clean && rm -rf  /var/lib/apt/lists/* && rm -f /etc/pbs.conf && chown -R ${USER_UID}:${USER_GID} /etc/ssh/* && \
       pip3 install configparser && rm -rf ~/.cache

RUN ln -fs /usr/share/zoneinfo/Europe/Prague /etc/localtime && dpkg-reconfigure --frontend noninteractive tzdata && sed -i -e 's/# en_US.UTF-8 UTF-8/en_US.UTF-8 UTF-8/' /etc/locale.gen && locale-gen

RUN /usr/sbin/addgroup --gid ${USER_GID} ${USER_GROUP} && \
    /usr/sbin/adduser --gid ${USER_GID} --uid ${USER_UID} --disabled-password --gecos ${USER_NAME} ${USER_NAME} && \ 
    mkdir /home/${USER_NAME}/.ssh && \
    chown ${USER_UID}:${USER_GID} /home/${USER_NAME}/.ssh

## setup SSHD to be able to run as user

RUN sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/g' /etc/ssh/sshd_config && \
 sed -i 's/.*ClientAliveInterval.*/ClientAliveInterval 30/g' /etc/ssh/sshd_config && \
 sed -i "s/#Port 22.*/Port ${SSH_PORT}/g" /etc/ssh/sshd_config && \
 sed -i 's/UsePAM.*/UsePAM no/g' /etc/ssh/sshd_config && \
 echo "PubkeyAcceptedKeyTypes=+ssh-rsa" >> /etc/ssh/sshd_config

WORKDIR /tmp

EXPOSE ${SSH_PORT}

RUN mkdir /etc/hpcconf && mkdir /etc/secrets && rm -f /etc/krb5.conf && ln -s /etc/hpcconf/pbs.conf /etc/pbs.conf && ln -s /etc/secrets/krb5.conf /etc/krb5.conf && ln -s /etc/secrets/krb5.keytab /etc/krb5.keytab

RUN wget https://dl.k8s.io/release/v${KUBERNETES_VERSION}/bin/linux/amd64/kubectl -O /usr/bin/kubectl && chmod a+rx /usr/bin/kubectl


RUN echo "#!/bin/bash\nkubectl get secret -n \${NAMESPACE} sshkey -o jsonpath='{.data.ssh-publickey}' | base64 -d > /home/${USER_NAME}/.ssh/authorized_keys || echo \"Failed to get ssh key.\"\n\nchmod 0600 /home/${USER_NAME}/.ssh/authorized_keys\n\nset | grep 'PROXY\|KUBERNETES' | sed -e 's/^/export /' >> /home/${USER_NAME}/.bashrc;\n\ntouch /tmp/sshd.log\n/usr/sbin/sshd -E /tmp/sshd.log\ntail -f /tmp/sshd.log\n" > /srv/start-sshproxy.sh && chmod a+rx /srv/start-sshproxy.sh

COPY --chown=${USER_UID} start.sh.tmpl /tmp/
COPY --chown=${USER_UID} run-qsub.sh.tmpl /tmp
COPY --chown=${USER_UID} run-qsub-gpu.sh.tmpl /tmp
COPY --chown=${USER_UID} genproxy.py /srv/
COPY --chown=${USER_UID} config.ini /srv/

RUN /bin/bash -c "export USER_NAME=$USER_NAME; envsubst '\$USER_NAME' < /tmp/run-qsub.sh.tmpl > /srv/run-qsub.sh && rm -f /tmp/run-qsub.sh.tmpl"
RUN /bin/bash -c "export USER_NAME=$USER_NAME; envsubst '\$USER_NAME' < /tmp/run-qsub-gpu.sh.tmpl > /srv/run-qsub-gpu.sh && rm -f /tmp/run-qsub-gpu.sh.tmpl"
RUN /bin/bash -c "export USER_NAME=$USER_NAME; envsubst '\$USER_NAME' < /tmp/start.sh.tmpl > /srv/start.sh && rm -f /tmp/start.sh.tmpl && chmod a+rx /srv/start.sh"

USER ${USER_UID}

CMD /bin/bash -c "set | grep 'PROXY\|KUBERNETES' | sed -e 's/^/export /' >> /home/${USER_NAME}/.bashrc; /usr/sbin/sshd -e; if [ -n \"$SSH_KEY\" ]; then echo \"$SSH_KEY\" > /home/${USER_NAME}/.ssh/authorized_keys; fi; tail -f /dev/null"
