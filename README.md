![AwestruckMarkSmall](https://user-images.githubusercontent.com/1250151/235605837-62579f30-4ad2-485a-a9dc-2c348ca4369f.png)

# Awestruck
Real-time synthesis and streaming with SuperCollider

**NOTE: this prototype currently broken. issues to come soon**

This is what it should be able to do: https://youtu.be/iEC6-pBFj2Q

## Getting Started
You'll need Docker/Docker Compose to build and run locally.

* make build
* make up
* localhost:8080

## Deployments
Docker is used to build an image, deploy to AWS ECR, and run within EB using ECS.

Make sure you set your AWS env vars according to .env.sample by creating your own .env or .envrc file.


## Vision
My original goal for Awestruck was to further open source music development and composition with SuperCollider. My thought was that if compositions could be synthesized and manipulated in realtime by separate collaborators/clients, that this might open up new possibilities for music.

### Next steps
* Fix the broken prototype – WebRTC is not connecting as it previously was. Likely version related since I am dusting this off several years later
* Support n SuperCollider instances and client connections. Handle disconnections gracefully
* Start collecting .scd submissions. Start by importing some of my old ones that live here: https://github.com/keypulsations/variations
* Explore how LLMs can be used to create and refine new compositions using realtime input from humans