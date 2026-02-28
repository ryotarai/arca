FROM cloudflare/cloudflared:latest AS cloudflared

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
  && apt-get install -y --no-install-recommends bash ca-certificates dumb-init python3 \
  && rm -rf /var/lib/apt/lists/* \
  && useradd --create-home --home-dir /home/arca --shell /bin/bash arca

COPY --from=cloudflared /usr/local/bin/cloudflared /usr/local/bin/cloudflared
COPY docker/machine/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

USER arca
WORKDIR /home/arca

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/local/bin/entrypoint.sh"]
