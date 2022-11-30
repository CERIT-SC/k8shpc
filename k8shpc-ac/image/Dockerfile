#TAG=30-11-1-2022
#PROJECT=cerit
FROM golang:alpine3.16
RUN apk update && apk add --no-cache bash vim procps
WORKDIR /home
COPY additional_files/k8shpc/main.go ./
COPY additional_files/k8shpc/go.mod ./
COPY additional_files/k8shpc/go.sum ./
RUN CGO_ENABLED=0 go build -o /usr/local/bin/k8shpc-mutating-webhook
RUN adduser --disabled-password --no-create-home --uid 1000 k8shpc
RUN chmod a+rwx /usr/local/bin && chmod a+rx /usr/local/bin/k8shpc-mutating-webhook
WORKDIR /home

USER 1000
ENTRYPOINT ["/usr/local/bin/k8shpc-mutating-webhook"]
