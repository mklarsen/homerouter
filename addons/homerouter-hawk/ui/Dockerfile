# Build homerouter-hawk-ui API server

FROM node:22-alpine

WORKDIR /app

# Install backend dependencies
COPY addons/homerouter-hawk/ui/package.json ./
RUN npm install --omit=dev && \
    npm install express express-basic-auth sqlite3

# Copy pre-built frontend dist
COPY addons/homerouter-hawk/ui/dist ./dist

# Copy backend server
COPY addons/homerouter-hawk/ui/server ./server

# Create data directory
RUN mkdir -p /data && chown -R node:node /data

USER node:node
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD node -e "const port = process.env.PORT || 3000; require('http').get(\`http://localhost:\${port}/api/health\`, (r) => {if (r.statusCode !== 200) throw new Error(r.statusCode)})"

ENTRYPOINT ["node", "server/api.js"]
