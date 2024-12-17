(
// A generative piece that balances structure and atmosphere.
// There's a sense of momentum through subtle rhythmic pulses,
// evolving harmonic patterns, and melodic fragments.
// Press play and let it evolve.

x = {
	// Declare all variables at the top
	var beatDur, chordDur, scale, baseMidi, chordRoots, chordTrig, chordIndex, currentRoot;
	var noteTrig, degree, freq, leadEnv, lead;
	var chordNotes, pad, padEnv;
	var bassTrig, bassEnv, bass;
	var percTrig, clickEnv, click, wash;
	var mix, dly;

	// Base rhythmic duration
	beatDur = 0.5;
	// How often chords change
	chordDur = 16;
	// Minor-ish scale degrees
	scale = [0, 2, 3, 5, 7, 9, 10];
	// Base note (C4)
	baseMidi = 60;
	// Randomly cycle through chord roots
	chordRoots = [0, 5, 7, 10].scramble;

	// Chord progression logic
	chordTrig = Impulse.kr(1 / chordDur);
	chordIndex = Stepper.kr(chordTrig, 0, 1, chordRoots.size, 1) - 1;
	currentRoot = Select.kr(chordIndex, chordRoots);

	// Generate a melodic pattern
	noteTrig = Impulse.kr(1 / beatDur);
	degree = Demand.kr(noteTrig, 0, Dshuf(scale, inf));
	freq = (baseMidi + currentRoot + degree).midicps * LFNoise1.kr(0.05).range(0.98, 1.02);

	// Melodic voice
	leadEnv = Decay2.kr(noteTrig, 0.01, 0.3);
	lead = Saw.ar(freq, 0.1) * leadEnv;
	lead = RLPF.ar(lead, freq * 4, 0.3).tanh;

	// Chord pad
	chordNotes = [0, 2, 5, 7].collect { |i| (baseMidi + currentRoot + i).midicps };
	pad = Mix(Saw.ar(chordNotes, 0.04));
	padEnv = EnvGen.kr(Env.asr(2, 1, 4), chordTrig, doneAction: 0);
	pad = RLPF.ar(pad, 800 * LFNoise1.kr(0.1).range(0.5, 2), 0.4) * padEnv.lag(0.1);
	pad = pad + (LFTri.ar(chordNotes.choose, 0, 0.02));
	pad = pad.tanh;

	// Bass pulse
	bassTrig = Impulse.kr(1 / (beatDur * 2));
	bassEnv = Decay2.kr(bassTrig, 0.01, 0.2);
	bass = SinOsc.ar((baseMidi + currentRoot - 12).midicps, 0, 0.2) * bassEnv;
	bass = LPF.ar(bass, 200) * 0.5;

	// Percussive layer
	percTrig = Impulse.kr(1 / (beatDur * 4));
	clickEnv = Decay2.kr(percTrig, 0.005, 0.05);
	click = HPF.ar(WhiteNoise.ar(0.02), 2000) * clickEnv;
	wash = CombL.ar(PinkNoise.ar(0.001), 0.1, 0.05, 1) * LFNoise1.kr(0.1).range(0.5, 1) * 0.2;

	// Mix all elements
	mix = lead + pad + bass + click + wash;

	// Spatialization
	mix = Rotate2.ar(mix, mix, LFNoise2.kr(0.1));

	// Add a gentle delay and reverb
	dly = CombL.ar(mix, 0.3, 0.3, 2);
	mix = mix + dly * 0.2;
	mix = GVerb.ar(mix, 70, 30, 0.3, drylevel: 0.5, earlyreflevel: 0.2, taillevel: 0.8, maxroomsize: 300).softclip;

	// Gentle limiting
	mix = Limiter.ar(mix, 1, 0.01);

	mix;
}.play;
)
