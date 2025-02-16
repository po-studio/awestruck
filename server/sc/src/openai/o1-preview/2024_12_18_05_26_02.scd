SynthDef.new("openai-o1-preview-2024_12_18_05_26_02", { |out=0, amp=0.5|
  var oscLayers, noiseLayers, percussiveElements;
  var modLFO1, modLFO2, modLFO3;
  var noiseMod1, noiseMod2, noiseMod3;
  var localIn, feedback, filters, waveshaped, pitchShifted;
  var delays, reverbs, modDelay, sound;
  var lfo1, lfo2, lfo3, env1, ampMod;
  var sequencer, demandFreq;
  
  //=====================================================================
  // MODULATION LFOs
  //=====================================================================
  modLFO1 = LFNoise1.kr(0.05).range(-5, 5);
  modLFO2 = LFNoise1.kr(0.03).range(-10, 10);
  modLFO3 = LFNoise1.kr(0.01).range(-3, 3);
  
  //=====================================================================
  // SEQUENCER FOR PITCHED ELEMENTS
  //=====================================================================
  sequencer = Dseq([60, 62, 64, 65, 67, 69, 71, 72].midicps, inf);
  demandFreq = Demand.kr(Impulse.kr(0.2), 0, sequencer);
  
  //=====================================================================
  // LFOs FOR OSCILLATOR MODULATION
  //=====================================================================
  lfo1 = LFSaw.kr(0.05).range(-5, 5);
  lfo2 = SinOsc.kr(0.03).range(-3, 3);
  lfo3 = LFNoise1.kr(0.01).range(-2, 2);
  
  //=====================================================================
  // OSCILLATOR LAYERS
  //=====================================================================
  oscLayers = Mix.new([
    SinOsc.ar(demandFreq + lfo1, 0, 0.1),
    Saw.ar(demandFreq * 0.5 + lfo2, 0.1),
    Pulse.ar(demandFreq * 2 + lfo3, 0.5, 0.1)
  ]);
  
  //=====================================================================
  // NOISE LAYERS
  //=====================================================================
  noiseMod1 = LFNoise1.kr(0.02).range(200, 6000);
  noiseMod2 = LFNoise1.kr(0.03).range(100, 5000);
  noiseMod3 = LFNoise1.kr(0.04).range(300, 7000);
  
  noiseLayers = Mix.new([
    RLPF.ar(PinkNoise.ar(0.1), noiseMod1, 0.4),
    BPF.ar(WhiteNoise.ar(0.1), noiseMod2, 0.2),
    HPF.ar(BrownNoise.ar(0.1), noiseMod3)
  ]);
  
  //=====================================================================
  // PERCUSSIVE ELEMENTS
  //=====================================================================
  percussiveElements = Mix.fill(3, {
    var trig, env, source, filterFreq, percussive;
    trig = Dust.kr(0.5);
    env = EnvGen.kr(Env.perc(0.001, 0.1, 1, -4), trig);
    source = WhiteNoise.ar * env;
    filterFreq = TRand.kr(600, 9000, trig);
    percussive = BPF.ar(source, filterFreq, 0.1);
    percussive;
  });
  
  //=====================================================================
  // FEEDBACK LOOP
  //=====================================================================
  localIn = LocalIn.ar(2);
  feedback = CombL.ar(localIn, 1, [0.8, 0.9], 4);
  LocalOut.ar(feedback);
  
  // Combine all sound elements
  sound = oscLayers + noiseLayers + percussiveElements + feedback;
  
  //=====================================================================
  // FILTERING
  //=====================================================================
  filters = sound;
  filters = RLPF.ar(filters, LFNoise1.kr(0.05).exprange(200, 5000), 0.5);
  filters = BPF.ar(filters, LFNoise1.kr(0.03).exprange(300, 6000), 0.3);
  filters = HPF.ar(filters, LFNoise1.kr(0.02).exprange(100, 2000));
  
  //=====================================================================
  // WAVESHAPING AND PITCH SHIFTING
  //=====================================================================
  waveshaped = tanh(filters) + (filters - (filters * filters * filters) * 0.1);
  pitchShifted = PitchShift.ar(waveshaped, 0.2, LFNoise1.kr(0.01).range(0.8, 1.2), 0, 0.05);
  
  //=====================================================================
  // DELAYS AND REVERBS
  //=====================================================================
  delays = CombL.ar(pitchShifted, 0.5, LFNoise1.kr(0.02).range(0.1, 0.4), 4);
  modDelay = AllpassN.ar(delays, 0.05, [0.02, 0.03], 2);
  
  reverbs = Mix.new([
    GVerb.ar(modDelay, 50, 8, 0.5, 0.5, 15, 0.7, 0.5, 0.5),
    FreeVerb.ar(modDelay, 0.7, 0.8, 0.5),
    CombL.ar(modDelay, 0.3, 0.2, 3)
  ]);
  
  //=====================================================================
  // ENVELOPE AND FINAL OUTPUT
  //=====================================================================
  env1 = EnvGen.kr(Env.linen(10, 50, 10, 1), doneAction: 2);
  ampMod = LFNoise0.kr(0.02).range(0.5, 1);
  
  sound = Mix.new(reverbs) * ampMod * env1;
  sound = PanAz.ar(2, sound, LFSaw.kr(0.1));
  sound = Limiter.ar(sound * 0.9);
  
  Out.ar(out, sound * amp);
}).writeDefFile("/app/sc/synthdefs");