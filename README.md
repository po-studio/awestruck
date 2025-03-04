![brandmark](client/public/BrandSquare.png)

# Awestruck

> [!WARNING]  
> Protect your ears! Before streaming audio, please turn the volume on your machine DOWN, especially if you're using headphones. While we've tried to ensure the examples play at a reasonable volume, this software gets close to audio hardware and rare glitches such as amplitude spikes can occur.

## Overview

Awestruck is a framework for real-time, server-driven audio synthesis, streaming, and control over the Internet. It enables complex audio processing and algorithmic composition through SuperCollider while streaming the results to web clients via WebRTC.

## Technical Architecture

### Server-Side Components

1. **SuperCollider Server (scsynth)**

   - Real-time audio synthesis engine
   - Runs headlessly in server environment
   - Processes audio at microsecond precision
   - Controlled via OSC (Open Sound Control)

2. **JACK Audio Connection Kit**

   - Low-latency audio routing
   - Connects SuperCollider to GStreamer
   - Professional-grade audio server
   - Sample-accurate timing

3. **GStreamer Pipeline**

   - Audio capture from JACK
   - Format conversion and encoding
   - Buffer management
   - Pipeline: `jackaudiosrc -> audioconvert -> opusenc -> webrtcbin`

4. **WebRTC Stack (Pion)**
   - Peer connection management
   - ICE/STUN/TURN handling
   - Audio streaming
   - Connection negotiation

### Connection Flow

```
title Awestruck Connection & Streaming Flow

Browser->Server:1. Request WebRTC connection
note over Server:2. Create GStreamer pipeline
Server->Browser:3. Send SDP offer
Browser->Server:4. Return SDP answer
note over Server,Browser:5. ICE candidate exchange
Server->SuperCollider:6. Start synth
SuperCollider->JACK:7. Audio output
JACK->GStreamer:8. Route audio
GStreamer->Browser:9. Stream via WebRTC
note over Browser:10. Audio playback
```

[View on SequenceDiagram.org ↗](https://sequencediagram.org/index.html#initialData=C4S2BsFMAIEEHdIGdgCcCuBjA1tAwgPYB2Rkmox0AZNAMpqQCGAtiEQObQBi4B8AUPwBCqPkkioAtAD5aEgG4SAXAEYAdNABKkAI7pkwaAHVIAI00AVPNEzFS5EMX5ECwGAUWo6C5QCYNeKhMbtAA4vRBLBLQAA4gMZDgbJD8cqieMiJiygDMGnJEACZ0ACIACtAEAGZVEsKi8OJSsj6oSgAsGtrA6KhEpRWMREiIqM6u7p7e6RIANFmNygCsGgCSeACiNkOFIIWMIZAAHpgAFkPsKWkZsugJqITgSYXKAGz5wIyohkgAnkTAU6pO4SR7PCQyABSsDwAGklAB2DSwdC7AiVdDAGKY-jQuEycIMKJtAAcXQImJgjFRjn4hMizAh0gWTSUAE4PgzoPIQIxjGZLHhxiEPNEWcoVAAGZE09ExcCMX6mRg4IA)

### Client-Server Interaction

```
title Client Control & Audio Flow

Client->Server:1. Send synthesis parameters
Server->SuperCollider:2. Translate to OSC commands
SuperCollider->SuperCollider:3. Process audio
SuperCollider->JACK:4. Output audio
JACK->GStreamer:5. Capture audio
GStreamer->WebRTC:6. Encode & stream
WebRTC->Client:7. Deliver audio
note over Client:8. Playback in browser
```

[View on SequenceDiagram.org ↗](https://sequencediagram.org/index.html#initialData=C4S2BsFMAIGFxJAdsOB7FAnN5oDJoBBAVwBMQ1oAxcNAdwCgH5EUBaAPgGVJMA3XgC4AjADpoPJKWgBnAJ4oAFpBkgZ0AA4BDTFoC2kYLxkMe-Xpy7ENvWDgSkhAJnEAVXUhngtR6MEoA8lyw0ADGaHp6WlImVjaYduAOFtzWtvYgjpiCAMziAArYoSrqWmQUpmkJGVmcAFKEsADSggAs4gHEwBpd0GXkaAwNzZwA4lzAmJD6QgCs4rBaGsDEU33lg+OT0waYnADqkABGAEqusIIAbOIAokjhjviy2-oMh6fnnCzIwIIA7OIACKQBACTDrAYMJBoXxoMFwBA-QQADgK3jkRy0oQA1tAQEhoEdsHQZLwgA)

## Future Directions

### LLM-Driven Synthesis

```
title LLM-Based Synth Generation

User->Server:1. "Create a ambient space music"
Server->LLM:2. Translate to musical intent
LLM->Server:3. Generate SuperCollider code
note over Server:4. Validate code safety
Server->SuperCollider:5. Load & compile
SuperCollider->Client:6. Stream result
Client->Server:7. Feedback/adjustments
```

[View on SequenceDiagram.org ↗](https://sequencediagram.org/index.html#initialData=C4S2BsFMAIBlYLIFoBCBDAzpAJtAygJ4B2wAFtAOKRGQBOaoA9kQFAsCqWtSAfHnQDc6ALgCMAOmgAiAMK1IDGGmhoAtgCMQ1YNAwAHNAGMYqgK4YQhqS360h3HvATCATJIAq9IhnCLowRmgzC0M0cGgQEm0WJ15be2EAZkkqGnpgGDxTPToZRnBwEGw6aENGYpYiRgzoRnt8QREAFkkANTCivzLi3TQAM0hgAhtGhyyc2jyCopEAVklYRjRcADJSxlU9ECgbbNz8wuKHGULtYQA2STxgeTVoeQxTcGAWE60SONHhAHZJADFIDh1EYANYAemWACtzMBVNoMEA)

### Asynchronous Composition Workflow

```
title Background Synthesis Generation

User->Server:1. Submit composition request
Server->Queue:2. Add to processing queue
note over Server:3. Process in background
Queue->LLM:4. Generate variations
LLM->SuperCollider:5. Test & validate
SuperCollider->Storage:6. Save successful results
Server->User:7. Notify completion
User->Server:8. Request playback
```

[View on SequenceDiagram.org ↗](https://sequencediagram.org/index.html#initialData=C4S2BsFMAICEEMDGBrA5gJwPYFcB2ATaAZQE9dgALSAZxGugHFJdJ15RNcAoLgVWtYBaAHxFWAN1YAuAIwA6YtgBGAWzDREmFQAdMtDrmjpIAR2w1gXMeknoRARXPmpAJgUBBfIWCZo2rIg0tLio0GaQ5ly4mMAwmLbEEtIAzAoACgFB0CCGSkhoWHj4XI4RkCIAMhUAslIALApMLGyx0OLw6CDsIJzUXFXVIkTY2qwAwpjg4CD40gCsCgAqFtAAZG3w0-jskFYj45NbQqI+bKiQUgBsCkTwktDU2IiB1NQAZtjgRjSfwH3WthE-GkAHYFAA5GIgN4kDRabRQAx8AR2URJdBSAAcCgASqZzNRgH5wPASHkUEA)

## Getting Started

### Prerequisites

> [OrbStack](https://orbstack.dev/) is recommended for Docker development.

- Docker / Docker Compose

### Quick Start

```bash
# Build the containers
make build

# Start the service
make up

# Visit localhost:8080 in your browser
```

### Graceful Shutdown

```bash
make down
```

## Deployments

Awestruck uses [CDK for Terraform](https://developer.hashicorp.com/terraform/cdktf) (CDKTF) to define and deploy its infrastructure as code.

### Infrastructure Components

1. **Container Services**

   - ECS Fargate clusters for WebRTC, TURN, and client services
   - Auto-scaling based on CPU/memory utilization
   - Task definitions with resource limits and health checks

2. **Networking**

   - VPC with public and private subnets
   - Application Load Balancer for client traffic
   - Network Load Balancer for WebRTC/TURN traffic
   - Security groups for service isolation

3. **DNS & TLS**
   - Route53 for DNS management
   - ACM certificates for TLS termination
   - Automatic certificate renewal

### Domain Configuration

> [!IMPORTANT]
> Before deploying, you must:
>
> 1. Own a domain and have it registered in AWS Route53
> 2. Store the domain in AWS Secrets Manager as `AWESTRUCK_DOMAIN`
> 3. TODO: the main.ts infra code does not yet implement this, so replace any "\*.awestruck.io"
> 4. Update the following DNS records in your domain:
>    - `app.$YOUR_DOMAIN` -> Client application
>    - `webrtc.$YOUR_DOMAIN` -> WebRTC server
>    - `turn.$YOUR_DOMAIN` -> TURN server

Example domain configuration in Secrets Manager:

```json
{
  "AWESTRUCK_DOMAIN": "yourdomain.com"
}
```

### Deployment Commands

```bash
# First-time setup
cd infra
npm install
cdktf get

# Deploy all services
make deploy-all

# Force redeploy (if services aren't updating)
make force-deploy-infra

# Destroy infrastructure
cd infra && npm run destroy
```

### Environment Variables

Copy `.env.sample` to `.env` and configure the following variables:

```bash
# Environment
export AWESTRUCK_ENV=development  # or production, w/e

# API Keys
export OPENAI_API_KEY=<your-openai-api-key>  # Required for LLM features, TBD
export AWESTRUCK_API_KEY=<some-secret-key>   # Your API key for auth

# TURN Server Configuration
export TURN_MIN_PORT=49152
export TURN_MAX_PORT=49252

# Note: HOST_IP is automatically set by the development scripts
```

### Infrastructure Costs

The deployment includes:

- ECS Fargate tasks
- Load Balancers (ALB & NLB)
- Route53 hosted zones
- ACM certificates
- CloudWatch logs

Estimated monthly cost: $50-100 USD (varies by region and usage)

## Development Roadmap

1. **Core Infrastructure**

   - Production deployment hardening
   - Error recovery mechanisms
   - Performance optimization
   - Connection stability improvements
   - Datastores...totally missing

2. **API Development**

   - RESTful endpoints for control
   - WebSocket real-time interface
   - Comprehensive API documentation
   - Client SDKs

3. **AI Integration**

   - LLM-powered synth generation
   - Text-to-speech streaming
   - Real-time audio manipulation
   - Collaborative composition tools

4. **Enterprise Features**

   - User authentication
   - Role-based access control
   - Usage analytics
   - Custom deployment options

5. **Music Quality**
   - Proabably the most important challenge
   - Would love ideas from the community!

## Contributing

Contributions are welcome! Please read our contributing guidelines and code of conduct before submitting pull requests.

## License

This project is licensed under the GNU General Public License v3.0 (GPL-3.0).

```
Copyright (C) 2024 Po Studio

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
```

The GPL-3.0 license ensures that:

- The software remains free and open source
- Any modifications or derivative works must also be released under GPL-3.0
- Users have the freedom to run, study, share, and modify the software
- Source code must be made available when distributing the software

For the full license text, see the [GNU GPL v3.0](https://www.gnu.org/licenses/gpl-3.0.en.html).
