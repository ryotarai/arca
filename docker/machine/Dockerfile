FROM cloudflare/cloudflared:latest AS cloudflared

FROM alpine:3.21

RUN apk add --no-cache ca-certificates dumb-init \
  && adduser -D -h /home/arca arca

COPY --from=cloudflared /usr/local/bin/cloudflared /usr/local/bin/cloudflared
COPY docker/machine/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

USER arca
WORKDIR /home/arca

EXPOSE 8080
ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/local/bin/entrypoint.sh"]
