
SynthDef.new("openai-gpt-4o-2024_12_17_08_54_20", { |out=0, amp=0.5|
    var sound;
    var tonal, noise, percussive, mod1, mod2, freq1, freq2, pitchShifted, filter1, filter2, reverb, delay, feedback, localIn, localOut, layered, evolving;
tonal = Mix.fill(3, { Ringz.ar(SinOsc.ar(SinOsc.kr(0.1, 0, 5, 7) * 3 + 440), 200, LFNoise1.kr(0.1).range(0.01, 0.1)) });
noise = RHPF.ar(WhiteNoise.ar(0.1), LFNoise1.kr(0.1).range(200, 5000), LFNoise1.kr(0.05).range(0.1, 0.5));
percussive = Dust.ar(2) * Decay.ar(Impulse.ar(LFNoise0.kr(0.2).range(0.2, 2)), 0.3) * 0.3;
mod1 = SinOsc.kr(0.1).range(0.001, 0.1);
mod2 = LFNoise1.kr(0.05).range(0.1, 0.5);
freq1 = XLine.kr(100, 2000, 30);
freq2 = XLine.kr(200, 4000, 45);
pitchShifted = PitchShift.ar(tonal, 0.2, LFNoise1.kr(0.2).range(0.5, 2), 0.1);
filter1 = RLPF.ar(noise, freq1, mod1);
filter2 = BPF.ar(percussive + tonal, freq2, mod2) * 0.5;
reverb = GVerb.ar(layered, roomsize: 40, revtime: 9, damping: 0.7, inputbw: 0.5, spread: 0.8, drylevel: 0.3, earlyreflevel: 0.3, taillevel: 0.7);
delay = CombL.ar(layered, 0.5, SinOsc.kr(0.1).range(0.01, 0.1), 8);
localIn = LocalIn.ar(2);
feedback = Limiter.ar(localOut * 0.1);
localOut = LocalOut.ar(Splay.ar([delay, filter1, filter2] + feedback));
layered = SinOsc.ar(pitchShifted) * 0.1 + [filter1, filter2];
evolving = Mix.fill(3, { Resonz.ar(LFNoise1.ar(LFNoise1.kr(0.05).range(100, 1000)), 200, 0.3) });
sound = Splay.ar(layered + evolving + localIn * 0.5 + reverb * 0.5);
    Out.ar(out, sound * amp);
}).writeDefFile("/app/supercollider/synthdefs");
