# Turbo

> **Fastest** and **cheapest** ~~decentralized~~ distributed HQ residential proxy network.


What to run a node? Go to [our website](https://turbo-node.vercel.app) or [Turbo Client Node](https://github.com/TurboNodes/client-node)

## Features

1. [x] Self-sourced IPs and node connection quality analysis for highest success rate
3. [x] Redis Auth & PUB/SUB (Credits, Stream for abnormal traffic detection model)
4. [ ] Skip-hop option: connect directly to the end node for extremely low latency


## Global architecture

```mermaid
flowchart TD
    User(User)
    subgraph Self-hosted [Self-hosted backend]
        ProxyServer[Proxy Server]
        Redis[(Redis Database & Streams)]
        AI[AI Traffic Analysis] 
    end
%%    subgraph Zero-Trust Zone 
        ClientNode[Client Node]
        NodeRunner(Node Runner)
%%    end
    PaymentGateway[Payment Gateway]
    Blockchain[(Crypto Blockchain)]
    TargetWebsite[Target Website]
    Website[Next.js Website]
    Supabase[(Supabase PostgreSQL)]

    User --> |Sends HTTP/S or SOCKS5 Requests| ProxyServer
    User --> |Buys Credits| PaymentGateway
    
    ProxyServer --> |Node data update| Supabase
    ProxyServer <--> |TLS-encrypted QUIC Messaging| ClientNode
    ProxyServer --> |Uses for Auth & Credits| Redis
    
    AI --> |Evaluates server connections| Redis

    ClientNode --> |Processes Requests To| TargetWebsite
    NodeRunner --> |Operates| ClientNode
    

    PaymentGateway --> |Processes Payments| Blockchain
    PaymentGateway --> |Updates Credits In| Redis

    Blockchain --> |Sends Rewards| NodeRunner
    
    Website --> |Provides Data| NodeRunner
    Supabase <--> |Realtime stream| Website
    
    
```


#### Score calculation

Your node score is based on two factors:
- $L$: Latency, capped on a range from 10ms to 500ms
- $R$: Reliability

$$
S = w_L \cdot L + w_R \cdot R
$$

Where $w_L =$ 40% , $w_R =$ 60%

### Self-host a Server Node

You're free to operate your own server for commercial use according to the [Apache 2.0 license](LICENSE).

Run server docker image with `docker-compose up` and connect client nodes.

For more information, see [Setting Up Development Environment](.github/CONTRIBUTING.md#setting-up-development-environment)


## System Design

See [Global Architecture](#global-architecture) for a high-level overview of the system.

### Traffic flow

```mermaid
sequenceDiagram
    participant SOCKS5_Client as User
    participant Proxy_Server as Server
    participant Proxy_Client as Node
    participant Internet as Internet

    SOCKS5_Client->>Proxy_Server: 1. SOCKS5/HTTP CONNECT request
    Proxy_Server->>Proxy_Client: 2. Forward dest. IP + TLS-encrypted payload via QUIC 
    Proxy_Client->>Internet: 3. Process request (TCP level) 
    
    Internet-->>Proxy_Client: 4. Return encrypted response
    Proxy_Client-->>Proxy_Server: 5. Send data via QUIC
    Proxy_Server-->>SOCKS5_Client: 6. Send back to SOCKS5 Client
```

## Network Access

Want to buy proxy access from our network for web-scraping, browser agents?

Join our [**Discord server**](https://discord.gg/ZqdvQkSEc7): Create a ticket or DM _@lished_ for more information.