FROM ubuntu:latest

RUN apt-get update && apt-get install -y bash curl build-essential gcc tzdata

COPY go_telegram_forwarder_bot /bin/go_telegram_forwarder_bot

RUN chmod +x /bin/go_telegram_forwarder_bot && mkdir -p /bin/configs/

WORKDIR /bin/

ENTRYPOINT ["/bin/go_telegram_forwarder_bot"]