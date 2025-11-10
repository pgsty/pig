FROM debian:13
USER root
WORKDIR /root/
CMD ["/bin/bash"]

RUN apt update && apt install -y ca-certificates curl && curl https://repo.pigsty.io/pig | bash