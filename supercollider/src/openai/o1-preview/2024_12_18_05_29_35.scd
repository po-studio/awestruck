
SynthDef.new("openai-o1-preview-2024_12_18_05_29_35", { |out=0, amp=0.5|
    var sound;
    var sound,
    lfo1, lfo2, lfo3,
    freq1, freq2, freq3,
    osc1, osc2, osc3,
    pitchedElements,
    noise,
    modFilter1, modFilter2,
    filteredNoise,
    percussiveTriggers,
    percussiveEnv,
    percussiveSig,
    percussiveImpulses,
    filteredPercussive,
    feedbackLoop,
    feedbackProcessed,
    feedbackMix,
    feedbackOutput,
    delay1,
    reverb;

// Pitched elements
lfo1 = LFNoise1.kr(0.05).range(-5, 5);
lfo2 = LFNoise1.kr(0.07).range(-7, 7);
lfo3 = LFNoise1.kr(0.09).range(-9, 9);

freq1 = 220 + lfo1;
freq2 = 330 + lfo2;
freq3 = 440 + lfo3;

osc1 = SinOsc.ar(freq1);
osc2 = Saw.ar(freq2) * 0.8;
osc3 = Pulse.ar(freq3, 0.4) * 0.6;

pitchedElements = Mix.new([osc1, osc2, osc3]) * 0.3;

// Noise textures
noise = PinkNoise.ar(0.2);
modFilter1 = LFNoise1.kr(0.02).range(300, 3000);
modFilter2 = LFNoise1.kr(0.01).range(0.1, 0.9);
filteredNoise = RLPF.ar(noise, modFilter1, modFilter2) * 0.5;

// Percussive impulses
percussiveTriggers = Dust.kr(LFNoise1.kr(0.005).range(0.1, 1));
percussiveEnv = EnvGen.kr(Env.perc(0.001, 0.2), percussiveTriggers);
percussiveSig = WhiteNoise.ar * percussiveEnv;
filteredPercussive = BPF.ar(percussiveSig, LFNoise1.kr(0.1).range(400, 8000), 0.5) * 0.4;

// Feedback loop
feedbackLoop = LocalIn.ar(2);
feedbackProcessed = CombL.ar(feedbackLoop, 0.5, LFNoise1.kr(0.05).range(0.01, 0.5), 4);
feedbackMix = Mix.new([feedbackProcessed, pitchedElements, filteredNoise, filteredPercussive]).tanh * 0.5;
feedbackOutput = Limiter.ar(feedbackMix, 0.95, 0.05);

LocalOut.ar(feedbackOutput);

// Effects
delay1 = DelayC.ar(feedbackOutput, 2, LFNoise1.kr(0.01).range(0.5, 1.5));
reverb = GVerb.ar(delay1, 70, 0.8, 0.5, 0.5, 15, 0.7, 0.5, 0.3);

// Final output
sound = reverb * 0.5;
    Out.ar(out, sound * amp);
}).writeDefFile("/app/supercollider/synthdefs");
