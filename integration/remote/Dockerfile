FROM debian:bookworm-slim@sha256:88200866dfff7ea7f5cbcb6ec7c8a701889efe6fe859fe64d6990e4b07ea4171

RUN apt-get update && apt-get install -y --no-install-recommends openssh-server ca-certificates curl procps \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --shell /bin/sh tester \
    && passwd -d tester \
    && mkdir -p /run/sshd /home/tester/.ssh \
    && chmod 700 /home/tester/.ssh \
    && chown -R tester:tester /home/tester \
    && rm -f /etc/ssh/ssh_host_*

ARG HERDR_VERSION=0.8.2
ARG TARGETARCH
RUN case "$HERDR_VERSION/$TARGETARCH" in \
      0.8.2/arm64) asset=aarch64; digest=f55610658e1c2e0d2aaef730b4b2ab885f7f8ba00285ab372bfb14f2e3d5b40d ;; \
      0.8.2/amd64) asset=x86_64; digest=976150a14d490c94b243ea2e1a7eb2dfb67f12e36b182db90936f6728e6aecf4 ;; \
      0.9.0/arm64) asset=aarch64; digest=9c8db20fb7e7427b138d5367113f1621ffd319f2f65d6f009e2594029115f0d2 ;; \
      0.9.0/amd64) asset=x86_64; digest=4fa1a01158dd8043da92d31b270780b0dcc10603038d9b61cac4d81ab63fb71f ;; \
      *) exit 1 ;; \
    esac \
    && curl --fail --location --proto '=https' --proto-redir '=https' "https://github.com/herdrdev/herdr/releases/download/v${HERDR_VERSION}/herdr-linux-${asset}" -o /usr/local/bin/herdr \
    && echo "$digest  /usr/local/bin/herdr" | sha256sum -c - \
    && chmod 755 /usr/local/bin/herdr \
    && /usr/local/bin/herdr --version

COPY sshd_config /etc/ssh/sshd_config
ENTRYPOINT ["/usr/sbin/sshd", "-D", "-e"]
