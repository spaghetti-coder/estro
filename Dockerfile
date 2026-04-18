FROM node:24-alpine

RUN apk add --no-cache dumb-init openssh-client

WORKDIR /app

COPY package*.json ./
RUN npm ci --omit=dev

COPY app.js ./
COPY public ./public

RUN addgroup -S app && adduser -S app -G app
USER app

EXPOSE 3000
CMD ["dumb-init", "node", "app.js"]
