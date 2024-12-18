
SynthDef.new("openai-o1-preview-2024_12_18_05_45_31", { |out=0, amp=0.5|
    var sound, layer1, layer2, layer3;
var osc1, osc2, osc3, osc4, osc5, osc6;
var freq1, freq2, freq3, freq4, freq5, freq6;
var modFreq1, modFreq2, modFreq3;
var noise1, noise2, noise3;
var percussive1, percussive2, percTrig1, percTrig2;
var lfo1, lfo2, lfo3, lfo4, lfo5;
var filter1, filter2, filter3, filter4, filter5, filterFreq1, filterFreq2, filterFreq3;
var delay1, delay2, delayTime1, delayTime2;
var reverb;
var localIn, localOut, feedback;
var ampEnv1, ampEnv2, ampEnv3;
var pan1, pan2, pan3;

// Layer 1: Slowly shifting pitched elements
modFreq1 = LFNoise1.kr(0.01).range(0.1, 0.5);
modFreq2 = LFNoise1.kr(0.02).range(0.1, 0.5);
modFreq3 = LFNoise1.kr(0.015).range(0.1, 0.5);

freq1 = LFNoise1.kr(modFreq1).range(100, 400);
freq2 = LFNoise1.kr(modFreq2).range(200, 500);
freq3 = LFNoise1.kr(modFreq3).range(150, 450);
freq4 = SinOsc.kr(0.05).range(250, 550);
freq5 = SinOsc.kr(0.08).range(300, 600);
freq6 = SinOsc.kr(0.06).range(350, 650);

osc1 = SinOsc.ar(freq1) * 0.1;
osc2 = Saw.ar(freq2) * 0.1;
osc3 = Pulse.ar(freq3, 0.3) * 0.1;
osc4 = SinOsc.ar(freq4) * 0.1;
osc5 = Saw.ar(freq5) * 0.1;
osc6 = Pulse.ar(freq6, 0.7) * 0.1;

ampEnv1 = EnvGen.kr(Env.perc(5, 10, amp: 1), doneAction: 0);
pan1 = LFNoise1.kr(0.05).range(-1, 1);

layer1 = (osc1 + osc2 + osc3 + osc4 + osc5 + osc6) * ampEnv1;
layer1 = Pan2.ar(layer1, pan1);

// Layer 2: Dense, evolving noise-based textures
noise1 = PinkNoise.ar() * 0.2;
noise2 = BrownNoise.ar() * 0.2;
noise3 = WhiteNoise.ar() * 0.2;

filterFreq1 = LFNoise1.kr(0.02).range(300, 5000);
filterFreq2 = LFNoise1.kr(0.015).range(500, 8000);
filterFreq3 = LFNoise1.kr(0.025).range(200, 7000);

filter1 = RLPF.ar(noise1, filterFreq1, 0.5);
filter2 = RHPF.ar(noise2, filterFreq2, 0.5);
filter3 = BPF.ar(noise3, filterFreq3, 0.5);

ampEnv2 = EnvGen.kr(Env.perc(10, 15, amp: 1), doneAction: 0);
pan2 = LFNoise1.kr(0.03).range(-1, 1);

layer2 = (filter1 + filter2 + filter3) * ampEnv2;
layer2 = Pan2.ar(layer2, pan2);

// Layer 3: Subtle percussive impulses
percTrig1 = Dust.ar(LFNoise1.kr(0.05).range(0.2, 0.5));
percTrig2 = Dust.ar(LFNoise1.kr(0.07).range(0.2, 0.5));

percussive1 = Decay.ar(percTrig1, 0.1, SinOsc.ar(LFNoise1.kr(1).exprange(300, 2000)) * 0.3);
percussive2 = Decay.ar(percTrig2, 0.1, WhiteNoise.ar() * 0.2);

percussive1 = RLPF.ar(percussive1, 800, 0.9);
percussive2 = BPF.ar(percussive2, 1200, 0.7);

ampEnv3 = EnvGen.kr(Env.perc(0.01, 0.5, amp: 1), percTrig1 + percTrig2, doneAction: 0);
pan3 = LFNoise1.kr(0.1).range(-1, 1);

layer3 = (percussive1 + percussive2) * ampEnv3;
layer3 = Pan2.ar(layer3, pan3);

// Feedback loop with LocalIn/LocalOut
localIn = LocalIn.ar(2);
feedback = CombL.ar(localIn, 0.1, LFNoise1.kr(0.05).range(0.005, 0.1), 15);
feedback = feedback * 0.7;
localOut = LocalOut.ar(feedback);

// Combining layers and feedback
sound = layer1 + layer2 + layer3 + feedback;

// Non-linear waveshaping and pitch shifting
sound = tanh(sound * 2);
sound = PitchShift.ar(sound, pitchRatio: LFNoise1.kr(0.02).range(0.8, 1.2), windowSize: 0.2, pitchDispersion: 0.01, timeDispersion: 0.01);

// Additional filtering
filterFreq3 = LFNoise1.kr(0.01).range(400, 6000);
sound = RLPF.ar(sound, filterFreq3, 0.5);

// Delays and spatial effects
delayTime1 = LFNoise1.kr(0.05).range(0.1, 0.5);
delayTime2 = LFNoise1.kr(0.03).range(0.2, 0.6);

delay1 = DelayL.ar(sound, 0.5, delayTime1) * 0.5;
delay2 = CombL.ar(sound, 0.6, delayTime2, 8) * 0.5;

sound = sound + delay1 + delay2;

// Reverb and spatialization
reverb = GVerb.ar(sound, 50, 8, 0.5, 0.5, 15, 0.7, 0.5, 300);
sound = sound + reverb;

// Amplitude limiting and output
sound = Limiter.ar(sound, 0.9);
    Out.ar(out, sound * amp);
}).writeDefFile("/app/supercollider/synthdefs");
