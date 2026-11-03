FROM alpine:latest

RUN apk add --no-cache bash curl jq sqlite

WORKDIR /app

COPY entrypoint.sh .
RUN chmod +x entrypoint.sh

# Database volume
VOLUME /data

# Health check - should respond to basic probe
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD [ -f /data/ip-tracker.db ] || exit 1

CMD ["./entrypoint.sh"]
