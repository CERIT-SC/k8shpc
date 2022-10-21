#TAG=19-10-2022-5-editable
#PROJECT=cerit
FROM golang:alpine3.16 
#AS builder
#RUN apk update && apk add --no-cache bash

RUN apk update && apk add --no-cache bash vim procps
WORKDIR /home
COPY additional_files/k8shpc/main.go ./
COPY additional_files/k8shpc/go.mod ./
COPY additional_files/k8shpc/go.sum ./
#RUN CGO_ENABLED=0 go build -o /usr/bin/k8shpc-mutating-webhook
RUN CGO_ENABLED=0 go build -o /usr/local/bin/k8shpc-mutating-webhook

#FROM golang:alpine3.16
#RUN apk update && apk add --no-cache bash vim procps
#COPY --from=builder /usr/bin/k8shpc-mutating-webhook /usr/local/bin/k8shpc-mutating-webhook
#COPY /usr/bin/k8shpc-mutating-webhook /usr/local/bin/k8shpc-mutating-webhook
RUN adduser --disabled-password --no-create-home --uid 1000 k8shpc
RUN chmod a+rwx /usr/local/bin && chmod a+rx /usr/local/bin/k8shpc-mutating-webhook
WORKDIR /home

USER 1000
CMD ["/bin/sh", "-c", "while true; do /usr/local/bin/k8shpc-mutating-webhook; sleep 1; done"]
#CMD ["/usr/local/bin/k8shpc-mutating-webhook"]
