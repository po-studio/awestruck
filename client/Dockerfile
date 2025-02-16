# Build stage
FROM node:20-alpine as build

WORKDIR /app

# Copy package files
COPY package*.json ./

# Install dependencies
RUN npm install --quiet

# Copy source code
COPY . .

# Build the application
RUN npm run build

# Development stage
FROM node:20-alpine as development

WORKDIR /app

# Copy package files and install dependencies
COPY package*.json ./
RUN npm install --quiet

# Copy source code
COPY . .

# Set development environment
ENV NODE_ENV=development
ENV VITE_DEBUG=true

# Expose Vite dev server port
EXPOSE 5173

# Start development server with proper host binding
CMD ["npm", "run", "dev:docker"]

# Production stage
FROM nginx:alpine as production

# Install envsubst
RUN apk add --no-cache gettext

# Copy built assets from build stage
COPY --from=build /app/dist /usr/share/nginx/html

# Copy nginx configuration as template
COPY nginx.conf /etc/nginx/conf.d/default.conf.template

# Copy entrypoint script
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

EXPOSE 5173

# Use the entrypoint script
ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"] 
