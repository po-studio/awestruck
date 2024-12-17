
SynthDef.new("openai-gpt-4o-2024_12_17_08_53_24", { |out=0, amp=0.5|
    var sound;
    var tonal1, tonal2, noise, percTrig, percussive, feedback, lfo1, lfo2, modulated, filtered, pitchShifted, nonlinear, reverb, delay, dynamic, evolving;
	tonal1 = Mix.fill(4, {SinOsc.ar(mul: LFNoise1.kr(0.1, 100, 200), add: LFNoise1.kr(0.05, 400, 600))});
	tonal2 = Mix.fill(4, {Saw.ar(LFNoise1.kr(0.1, 100, 200), LFNoise0.kr(0.1).range(0.1, 0.3))});
	noise = BrownNoise.ar() * 0.1;
	percTrig = Dust.kr(LFNoise0.kr(0.3).exprange(0.1, 5));
	percussive = Decay.ar(percTrig, 0.3, WhiteNoise.ar() * LFNoise1.kr(1).range(0, 1));
	feedback = LocalIn.ar(2, 0) * 0.8;
	lfo1 = LFNoise1.kr(0.05).range(0.2, 0.8);
	lfo2 = LFNoise2.kr(0.07).range(100, 4000);
	modulated = SinOsc.ar(lfo2, mul: LFNoise1.kr(0.03).range(0.1, 0.25));
	filtered = RLPF.ar(tonal1 + tonal2 + modulated + feedback, lfo2, 0.2);
	pitchShifted = PitchShift.ar(filtered, 0.05, LFNoise1.kr(0.1).range(0.95, 1.05));
	nonlinear = tanh(pitchShifted * LFNoise1.kr(0.2).range(0.3, 1));
	reverb = GVerb.ar(nonlinear, 200, 8, 0.1, 0.5, 15, 0.3, 0.7, 0.5);
	delay = CombL.ar(reverb, LFNoise0.kr(0.05).range(0.1, 0.5), 0.3, 8);
	dynamic = SinOsc.kr(0.01, 0, 0.5, 0.5) * 0.9 + 0.1;
	evolving = EnvGen.ar(Env.asr(20, 1, 20), line.kr(0, 1, 120), doneAction: 2);
	sound = Limiter.ar(Splay.ar([delay + noise + percussive, spectral].sum, spread: 0.9) * dynamic * evolving, 0.9);
	LocalOut.ar(sound);
    Out.ar(out, sound * amp);
}).writeDefFile("/app/supercollider/synthdefs");
