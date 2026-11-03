# Multi-stage build for homerouter-hawk UI

# Stage 1: Build React frontend
FROM node:22-alpine AS frontend-builder

WORKDIR /app/ui
COPY ui/package.json ui/package-lock.json* ./
RUN npm ci

COPY ui/src ./src
COPY ui/public ./public
COPY ui/vite.config.js ./

RUN npm run build

# Stage 2: Build backend + combine
FROM node:22-alpine

WORKDIR /app

# Install dependencies for backend
COPY ui/package.json ui/package-lock.json* ./
RUN npm ci --only=production && \
    npm install express express-basic-auth sqlite3

# Copy built frontend
COPY --from=frontend-builder /app/ui/dist ./dist

# Copy backend
COPY ui/server ./server

# Create data directory
RUN mkdir -p /data && chown -R node:node /data

USER node:node
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD node -e "require('http').get('http://localhost:3000/api/health', (r) => {if (r.statusCode !== 200) throw new Error(r.statusCode)})"

ENTRYPOINT ["node", "server/api.js"]
