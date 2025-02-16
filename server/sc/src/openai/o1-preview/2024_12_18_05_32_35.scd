SynthDef.new("openai-o1-preview-2024_12_18_05_32_35", { |out=0, amp=0.5|
  var sound, pitchLayer, noiseLayer, percLayer, feedbackLayer;
  var freq1, freq2, freq3, freqMod1, freqMod2, freqMod3;
  var noiseFilterFreq, noiseMod, noiseEnv;
  var percTrig, percDur, percEnv, percFreq, percSignal;
  var feedbackSignal, feedbackDelay, feedbackMix;
  var lfo1, lfo2, lfo3, modIndex1, modIndex2, modIndex3;
  var filter1, filter2, filter3, filterFreq1, filterFreq2, filterFreq3;
  var reverbSignal, delaySignal;
  var pan1, pan2, pan3;
  
  // Frequency Modulation for Pitched Elements
  freqMod1 = LFNoise1.kr(0.1).range(0.995, 1.005);
  freqMod2 = LFNoise1.kr(0.07).range(0.993, 1.007);
  freqMod3 = LFNoise1.kr(0.05).range(0.990, 1.010);
  
  freq1 = 220 * freqMod1;
  freq2 = 330 * freqMod2;
  freq3 = 440 * freqMod3;
  
  // LFOs for Modulation Indices
  lfo1 = LFNoise0.kr(0.02).range(0.8, 1.2);
  lfo2 = LFNoise0.kr(0.015).range(0.7, 1.3);
  lfo3 = LFNoise0.kr(0.01).range(0.6, 1.4);
  
  // Modulation Indices
  modIndex1 = SinOsc.kr(0.1, 0, lfo1, 1);
  modIndex2 = SinOsc.kr(0.08, 0, lfo2, 1);
  modIndex3 = SinOsc.kr(0.06, 0, lfo3, 1);
  
  // Pitched Elements
  pitchLayer = Mix.new([
    Saw.ar(freq1 * modIndex1),
    SinOsc.ar(freq2 * modIndex2),
    Pulse.ar(freq3 * modIndex3, 0.5)
  ]);
  
  // Filters for Pitched Elements
  filterFreq1 = LFNoise1.kr(0.05).range(200, 2000);
  filterFreq2 = LFNoise1.kr(0.03).range(300, 2500);
  filterFreq3 = LFNoise1.kr(0.02).range(400, 3000);
  
  filter1 = RLPF.ar(pitchLayer, filterFreq1, 0.5);
  filter2 = BPF.ar(filter1, filterFreq2, 0.3);
  filter3 = HPF.ar(filter2, filterFreq3);
  
  // Noise Layer
  noiseMod = LFNoise1.kr(0.1).range(500, 5000);
  noiseFilterFreq = XLine.kr(5000, 200, 300);
  noiseEnv = EnvGen.kr(Env.perc(0.5, 10), Done.freeSelf, timeScale: 1);
  
  noiseLayer = RLPF.ar(
    PinkNoise.ar() * noiseEnv,
    noiseFilterFreq * noiseMod,
    0.4
  );
  
  // Percussive Layer
  percTrig = Dust.kr(0.5);
  percDur = LFNoise1.kr(0.2).range(0.05, 0.5);
  percEnv = EnvGen.kr(Env.perc(0.001, percDur), percTrig);
  percFreq = LFNoise1.kr(0.1).range(300, 800);
  
  percSignal = Resonz.ar(
    WhiteNoise.ar() * percEnv,
    percFreq,
    0.1
  );
  
  // Feedback Layer
  feedbackDelay = CombL.ar(LocalIn.ar(2), 0.5, 0.5, 4);
  feedbackMix = Mix.new([filter3, noiseLayer, percSignal, feedbackDelay]) * 0.8;
  LocalOut.ar(feedbackMix);
  
  // Reverb and Delay
  reverbSignal = GVerb.ar(feedbackMix, 50, 8, mul: 0.5);
  delaySignal = DelayL.ar(reverbSignal, 0.8, 0.8);
  
  // Panning
  pan1 = Pan2.ar(delaySignal, LFNoise1.kr(0.1).range(-1, 1));
  pan2 = Pan2.ar(reverbSignal, LFNoise1.kr(0.07).range(-1, 1));
  pan3 = Pan2.ar(feedbackMix, LFNoise1.kr(0.05).range(-1, 1));
  
  // Final Mix
  sound = Limiter.ar(Mix.new([pan1, pan2, pan3]) * 0.5, 0.9, 0.01);
  Out.ar(out, sound * amp);
}).writeDefFile("/app/sc/synthdefs");