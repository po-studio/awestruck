
SynthDef.new("openai-gpt-4o-2024_12_17_08_56_18", { |out=0, amp=0.5|
    var sound;
    var tonal1, tonal2, noise1, noise2, percussive, feedback, lfo1, lfo2, lfo3, lfo4, env1, env2, env3, pitchShifted, filtered, waveshaped, reverb, delay, spatialized;

lfo1 = LFNoise2.kr(0.1).range(0.2, 1.2);
lfo2 = LFNoise1.kr(0.03).range(400, 2000);
lfo3 = LFNoise1.kr(0.02).range(0.1, 0.3);
lfo4 = LFNoise2.kr(0.01).range(0, 1);

tonal1 = Saw.ar([120, 150] * lfo1) * 0.2;
tonal2 = SinOsc.ar([60, 90].poll, mul: 0.3);
noise1 = WhiteNoise.ar() * EnvGen.kr(Env.perc(0.01, 1), Impulse.kr(0.7), doneAction: Done.freeSelf);
noise2 = BPF.ar(WhiteNoise.ar(), lfo2, 0.2) * 0.3;
percussive = Dust.ar(1) * Decay.ar(Impulse.ar(0.5), 0.2) * 0.2;

feedback = LocalIn.ar(2);
feedback = (feedback + [tonal1, tonal2, noise1, noise2, percussive]) * 0.8;

filtered = RLPF.ar(feedback, lfo2, 0.5) + HPF.ar(feedback, 400) * 0.3;
pitchShifted = PitchShift.ar(filtered, pitchRatio: LFNoise1.kr(0.1).range(0.5, 2), timeDispersion: 0.05);
waveshaped = tanh(pitchShifted);

reverb = FreeVerb.ar(waveshaped, lfo3, 0.5, lfo4);
delay = CombL.ar(reverb, 0.5, lfo3 * 0.2, 1);

spatialized = Splay.ar(delay, spread: 0.7);
env1 = EnvGen.kr(Env.perc(0.01, 5), Impulse.kr(0.1));
env2 = EnvGen.kr(Env.perc(1, 10), Impulse.kr(0.05));
env3 = EnvGen.kr(Env.adsr(0.1, 0.5, 0.3, 2));

LocalOut.ar(spatialized);
sound = (spatialized * env1 * env2 * env3).softclip;
    Out.ar(out, sound * amp);
}).writeDefFile("/app/supercollider/synthdefs");
