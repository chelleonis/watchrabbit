API Handlers:

http: 

rabbitmq:  
Message flow:
1. Producers: create messages and send them to RabbitMQ
2. Exchanges: receive messages and route them to queues based on rules
3. Queues: store messages until they can be processed
4. Consumers: connect to queues and receive messages for processing  

Exchanges/Routing:  
Messages are sent based on different types of "exchanges":
Direct - exact matches 
Topic - Pattern matching w/ wildcards
Fanout - Broadcast to all bound queues
Headers - Route based on message header

Basics:
1. Choose RabbitMQ client 
2. Connection & Channel Management 
3. Declare Exchanges and Queues
4. Publishing Events
5. Consuming events 

Misc Notes:
Work Queues - each message is processed by exactly one working - tasks that need to be processed once by a free worker
Pub/Sub - each message is broadcast to all subscribers - for event notifications for services

Event flow - 
1. File design - new/changed files
2. Event Publication - Fileready event to RabbitMQ
3. Event Routing - RabbitMQ routes event to appropriate analysis queues
4. Analysis Execution - Works consume events & process files
5. Result publication - Workers publish AnalysisComplete events
6. Result Storage - Result aggregator stores analysis results
7. Client notification: 