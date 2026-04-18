FROM node:24-alpine

RUN apk add --no-cache dumb-init openssh-client shadow su-exec

WORKDIR /app

COPY package*.json ./
RUN npm ci --omit=dev

COPY app.js ./
COPY public ./public
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 3000
ENTRYPOINT ["/entrypoint.sh"]
CMD ["dumb-init", "node", "app.js"]
