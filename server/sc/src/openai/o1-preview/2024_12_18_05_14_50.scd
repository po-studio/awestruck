SynthDef.new("openai-o1-preview-2024_12_18_05_14_50", { |out=0, amp=0.5|
  var sound;
  var pitchedElements, noiseTextures, percussiveImpulses;
  var freqs, amps, oscSigs, lfoFreqs, lfoAmps;
  var noiseSig, noiseModFreqs;
  var percTrig, percEnv, percSig, percFilterFreq;
  var feedbackSig, filters, delaySig, delayTimes, reverbSig, shaperSig, env;
  
  //=====================================================================
  // PITCHED ELEMENTS
  //=====================================================================
  // Generate a set of frequencies with subtle random modulation
  freqs = [220, 275, 330, 440, 550] * LFNoise1.kr(0.01).exprange(0.5, 2);
  
  // Amplitudes with slight variations
  amps = LFNoise0.kr([0.1, 0.15, 0.2, 0.25, 0.3]).range(0.1, 0.3);
  
  // LFO frequencies and amplitudes for modulation
  lfoFreqs = LFNoise1.kr([0.005, 0.006, 0.007, 0.008, 0.009]).exprange(0.1, 0.5);
  lfoAmps = LFNoise0.kr([0.02, 0.025, 0.03, 0.035, 0.04]).range(0.8, 1.2);
  
  // Generate a mix of modulated sine waves
  oscSigs = Mix.fill(5, { |i|
    SinOsc.ar(freqs[i] * SinOsc.kr(lfoFreqs[i]).range(0.9, 1.1)) * amps[i] * lfoAmps[i]
  });
  pitchedElements = oscSigs;
  
  //=====================================================================
  // NOISE TEXTURES
  //=====================================================================
  // Modulate noise frequencies
  noiseModFreqs = LFNoise1.kr([0.01, 0.015]).exprange(100, 5000);
  
  // Bandpass and low-pass filtered noise
  noiseSig = Mix([
    BPF.ar(PinkNoise.ar(), noiseModFreqs[0], LFNoise0.kr(0.05).range(0.1, 0.5)),
    RLPF.ar(WhiteNoise.ar(), noiseModFreqs[1], LFNoise1.kr(0.02).range(0.1, 0.9))
  ]) * 0.2;
  noiseTextures = noiseSig;
  
  //=====================================================================
  // PERCUSSIVE IMPULSES
  //=====================================================================
  // Random percussive trigger
  percTrig = Dust.kr(LFNoise0.kr(0.1).range(0.1, 0.5));
  
  // Percussive envelope
  percEnv = EnvGen.ar(Env.perc(0.01, 0.2), percTrig);
  
  // Percussive signal with random filter frequency
  percFilterFreq = LFNoise1.kr(0.05).range(400, 1200);
  percSig = SinOsc.ar(percFilterFreq) * percEnv;
  percSig = RLPF.ar(percSig, percFilterFreq * LFNoise0.kr(0.1).range(0.5, 2), 0.9);
  percussiveImpulses = percSig * 0.5;
  
  //=====================================================================
  // FEEDBACK LOOP
  //=====================================================================
  feedbackSig = LocalIn.ar(2);
  feedbackSig = feedbackSig + pitchedElements + noiseTextures + percussiveImpulses;
  feedbackSig = feedbackSig.tanh * 0.99;
  LocalOut.ar(feedbackSig);
  
  //=====================================================================
  // FILTERING
  //=====================================================================
  filters = [RLPF, BPF, HPF].collect { |filter, i|
    filter.ar(feedbackSig, LFNoise1.kr(0.01 + i * 0.005).exprange(200, 2000), LFNoise0.kr(0.02 + i * 0.005).range(0.1, 0.9))
  };
  filters = Mix(filters) * 0.5;
  
  //=====================================================================
  // DELAY AND REVERB
  //=====================================================================
  delayTimes = LFNoise1.kr([0.05, 0.06]).range(0.1, 0.5);
  delaySig = CombL.ar(filters, 0.5, delayTimes, LFNoise0.kr(0.01).range(0.5, 2));
  
  reverbSig = GVerb.ar(delaySig, 50, 8, 0.5, 0.5, 15, 0.7, 0.5, 0.5);
  
  //=====================================================================
  // PITCH SHIFTING AND SHAPING
  //=====================================================================
  shaperSig = PitchShift.ar(reverbSig, 0.2, LFNoise1.kr(0.01).range(0.95, 1.05), 0, 0.01);
  shaperSig = tanh(shaperSig) * 0.5;
  
  //=====================================================================
  // ENVELOPE AND FINAL OUTPUT
  //=====================================================================
  env = EnvGen.kr(Env.linen(60, 600, 60, 1, \sin));
  sound = Limiter.ar(shaperSig * env, 0.5);
  
  Out.ar(out, sound * amp);
}).writeDefFile("/app/sc/synthdefs");