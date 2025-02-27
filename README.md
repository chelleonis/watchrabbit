WatchRabbit is an event-driven GO-backend with RabbitMQ integration.
The backend is built to scale and host concurrent users with different-levels of computational intensity and aims to assist the automation of many biomarker analysis related tasks for webapps

## Core Services
1. File Watcher - Monitor file changes and trigger analysis workflows  
2. Analysis Worker Services - Processes biomarker files and generates results  
3. Result Storage - Manages storage and retrieval of results  
4. API Gateway - Provides HTTP endpoints for clients to interact with the system  

## Infrastructure
1. RabbitMQ - Message broker for event-driven communication  
2. S3- Storage for analysis results  
3. Redis - LRU cache for frequently accessed results  
4. Docker - Containerization for consistency  
5. Kubernetes - Scaling for docker containers  

Project Structure:  
cmd/ - Application Entry Points (execution)
internal/ - Internal packages
- config/ configuration management
- domain/ - domain models
- services/ - business logic
- transport/ - API handlers
pkg/ - Reusable Packages
deployments/ - Infrastructure files related to deployment of the backend
scripts/ - Utility scripts (TBD if needed)

Projected Project Timeline:
Stage 1 - Minimum Viable Product
Simple Go server watching for data changes
Basic R markdown rendering on triggers
Single Server deployment
Basic error logging to files
(Data -> Go watcher -> R Script -> output html)

Stage 2 - production ready
Docker containerization
Basic health checks
Simple automated deploment (github actions)
Redis for concurrency

Data -> Go service (+basic logs) -> Redis -> R container -> S3/Storage

Stage 3 - Team Scale 
    - Multiple Instances
    - Basic mOnitoring
    - Load Balancer
    - Proper error handling

Users -> LB -> (mulitple app instances + monitoring) -> R containers
( nginx, prometheus (metrics), error handling, user auth)

Stage 4 - enterprise -
Kubernetes
Full monitoring,
Automated scaling
Full CI/CD

Users -> K8s cluster -> multiple pods -> message queue -> R workers
(k8s cluster will have full monitoring, alerting system, + auto scaling/disaster recovery)