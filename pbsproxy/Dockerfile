FROM debian:11

RUN apt-get update && DEBIAN_FRONTEND=noninteractive  apt-get -y --no-install-recommends install ed gettext-base procps openssh-server vim bind9-host wget tzdata locales libgssapi3-heimdal libedit2 libpython3.9 libtcl8.6 libtk8.6 python krb5-user openssh-client && wget http://repo.cerit-sc.cz/debian/pool/main/libpbspro_19.0.0-28%2Bdeb11_amd64.deb && dpkg -i libpbspro_19.0.0-28+deb11_amd64.deb && wget http://repo.cerit-sc.cz/debian/pool/main/pbspro-client_19.0.0-28%2Bdeb11_amd64.deb && dpkg -i pbspro-client_19.0.0-28+deb11_amd64.deb && rm -f *.deb && apt-get clean && rm -rf  /var/lib/apt/lists/* && rm -f /etc/pbs.conf && chown -R 1000:1000 /etc/ssh/*;

RUN ln -fs /usr/share/zoneinfo/Europe/Prague /etc/localtime && dpkg-reconfigure --frontend noninteractive tzdata && sed -i -e 's/# en_US.UTF-8 UTF-8/en_US.UTF-8 UTF-8/' /etc/locale.gen && locale-gen

RUN /usr/sbin/addgroup --gid 1000 meta &&  /usr/sbin/adduser --gid 1000 --uid 1000 --disabled-password --gecos funnelworker funnelworker && mkdir /home/funnelworker/.ssh && chown 1000:1000 /home/funnelworker/.ssh

RUN sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/g' /etc/ssh/sshd_config && \
 sed -i 's/.*ClientAliveInterval.*/ClientAliveInterval 30/g' /etc/ssh/sshd_config && \
 sed -i 's/#Port 22.*/Port 2222/g' /etc/ssh/sshd_config && \
 sed -i 's/UsePAM.*/UsePAM no/g' /etc/ssh/sshd_config && \
 echo "PubkeyAcceptedKeyTypes=+ssh-rsa" >> /etc/ssh/sshd_config

WORKDIR /tmp

EXPOSE 2222

RUN mkdir /etc/secrets && rm -f /etc/krb5.conf && ln -s /etc/secrets/pbs.conf /etc/pbs.conf && ln -s /etc/secrets/krb5.conf /etc/krb5.conf && ln -s /etc/secrets/krb5.keytab /etc/krb5.keytab

RUN wget --no-check-certificate https://dl.k8s.io/release/v1.22.0/bin/linux/amd64/kubectl -O /usr/bin/kubectl && chmod a+rx /usr/bin/kubectl

USER 1000

COPY test.sh /home/funnelworker/test.sh

COPY start.sh /srv/
COPY run-qsub.sh /srv/
COPY service.yaml /srv/
COPY ssh-proxy.yaml /srv/

CMD /bin/bash -c "set | grep 'PROXY\|KUBERNETES' | sed -e 's/^/export /' >> /home/funnelworker/.bashrc; /usr/sbin/sshd; if [ -n \"$SSH_KEY\" ]; then echo \"$SSH_KEY\" > /home/funnelworker/.ssh/authorized_keys; fi; tail -f /dev/null"
