title Awestruck Audio Generation and Streaming Flow

User->Browser:Click "Generate Synth"
Browser->ALB:POST /generate\n{prompt: "user's text"}
ALB->WebRTC Server:Forward request

note over WebRTC Server:1. Generate unique sessionID\n2. Allocate UDP port (10000-10010)\n3. Initialize WebRTC peer

WebRTC Server->Audio Generator:Start audio generation task
note over Audio Generator:Generate audio based on\nprompt using ML model

WebRTC Server->Browser:Return {sessionID, ICE config}

Browser->Browser:Create RTCPeerConnection\nwith TURN credentials

Browser->TURN Server:STUN binding request
TURN Server->Browser:STUN binding response

Browser->NLB:TURN allocate request (UDP/3478)
NLB->TURN Server:Forward allocate request

note over TURN Server:1. Bind to container IP\n2. Allocate relay address\n3. Advertise NLB IP to client

TURN Server->Browser:Relay address (NLB IP)

Browser->WebRTC Server:POST /offer\n{sdp, ICE candidates}
WebRTC Server->Browser:Return {answer sdp}

note over Browser,WebRTC Server:ICE Connectivity Checks

Audio Generator->WebRTC Server:Audio chunks ready
WebRTC Server->NLB:Stream audio (UDP/10000-10010)
NLB->Browser:Forward audio stream

note over Browser:Play audio through\nWebAudio API

Browser->WebRTC Server:ICE candidate updates
WebRTC Server->Browser:ICE candidate updates

note over Browser,WebRTC Server:Continuous audio streaming\nuntil completion