FROM alpine

RUN apk update && apk add ca-certificates

COPY kube_initializer /
RUN chmod +x /kube_initializer

ENTRYPOINT ["./kube_initializer"]
