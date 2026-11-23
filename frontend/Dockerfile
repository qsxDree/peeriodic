# --- Build Stage ---
FROM node:20-alpine AS builder

WORKDIR /app

# Copy dependency definitions and npm configuration
COPY client/package*.json client/.npmrc ./

# Install dependencies
RUN npm ci --legacy-peer-deps

# Copy client source code
COPY client/ ./

# Build production bundle
RUN npm run build

# --- Runtime Stage ---
FROM nginx:1.27-alpine

# Copy custom Nginx configuration
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Copy compiled static assets from builder
COPY --from=builder /app/build /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:80/ || exit 1

CMD ["nginx", "-g", "daemon off;"]
