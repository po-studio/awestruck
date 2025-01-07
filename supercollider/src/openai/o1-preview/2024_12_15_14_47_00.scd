SynthDef.new("openai-o1-preview-2024_12_15_14_47_00", { |out=0, amp=0.5|
  var pad, padEnv, padFreqs, padSig, bd, sd, bdTrig, sdTrig, main, fb, pit, seq, demandTrig, res, resonanceF, drumMix, noiseEnv;
  var padFilt, shimmer, rev, diff, continuousEnv, bassPulse, snareNoise;
  
  // Basic continuous envelope to shape overall movement
  continuousEnv = EnvGen.kr(Env.asr(10, 1, 20, curve:-4), Impulse.kr(1/128), doneAction:0);
  
  // Sequence and triggers
  demandTrig = Impulse.kr(1/4, 0.5);
  seq = Demand.kr(demandTrig, 0, Dseq([60, 67, 63, 70, 55, 72, 76, 80], inf));
  // Resonance frequency follows the sequence
  resonanceF = seq.midicps;
  
  // Pad frequencies - a stack of shifting frequencies for a pad
  padFreqs = (1..5)*LFNoise2.kr(0.05).range(150,400);
  padEnv = EnvGen.kr(Env.perc(2,10,1,-4), Impulse.kr(1/64));
  padSig = Mix({ |i|
    var osc = Saw.ar(padFreqs[i]) * 0.1;
    var filtered = RLPF.ar(osc, LFNoise1.kr(0.1).range(500,2000), 0.5);
    filtered
  } ! 5) * padEnv * continuousEnv;
  padSig = padSig + (Formant.ar(LFNoise1.kr(0.05).range(50,120), LFNoise1.kr(0.05).range(600,900), 800)*0.05);
  
  // Add a bit of resonance filtering to the pad
  padFilt = RLPF.ar(padSig, resonanceF, 0.2 + LFNoise1.kr(0.5).range(-0.1,0.1));
  
  // Bass drum
  bdTrig = Dust.kr(1/4); // a BD hit roughly every 4 seconds
  bd = EnvGen.kr(Env.perc(0.001,0.3,1,-4), bdTrig) * SinOsc.ar(50,0)*0.8;
  bd = bd + Ringz.ar(Impulse.ar(bdTrig), 60, 0.15, 0.5); // Add a bit of ring for body
  
  // Snare drum
  sdTrig = Dust.kr(1/8); // snare roughly every 2 seconds or so
  snareNoise = WhiteNoise.ar(0.1) * EnvGen.kr(Env.perc(0.001,0.2,1,-4), sdTrig);
  sd = Ringz.ar(snareNoise, 2000,0.07,0.3);
  sd = sd + RLPF.ar(snareNoise, 3300,0.5)*0.2;
  
  drumMix = bd + sd;
  
  // Local feedback chain for shimmer/pitch-shift
  fb = LocalIn.ar(2);
  fb = HPF.ar(fb,60);
  pit = PitchShift.ar(fb, 0.5, 1.2, 0,0.01,0.8) 
  + PitchShift.ar(fb,0.5,0.9,0,0.01,0.95);
  pit = tanh(pit*1.4);
  LocalOut.ar(pit);
  
  // Add everything together
  main = padFilt + (pit*0.3) + drumMix*0.4;
  
  // Add a touch of resonant effects: a comb with seq-based delay
  main = main + CombL.ar(main,0.2,1/(resonanceF/200),2)*0.1;
  
  // Some evolving noise-based events
  noiseEnv = EnvGen.kr(Env.perc(0.001,0.5,1,-4), Dust.kr(0.1));
  main = main + Ringz.ar(PinkNoise.ar(0.02), resonanceF*2,0.2, noiseEnv*0.3);
  
  // Spatialization and diffusion
  main = Rotate2.ar(main, main, LFNoise1.kr(0.3));
  rev = GVerb.ar(main,50,30,0.2,0.5,0.7,0.5,0.3,0.9);
  diff = AllpassC.ar(rev,0.05,0.03,1) + AllpassC.ar(rev,0.05,0.02,1);
  main = (rev*0.7 + diff*0.3);
  
  // Filtering & limiting
  main = HPF.ar(main,40);
  main = LPF.ar(main,8000);
  main = Limiter.ar(main,1,0.01);
  
  main*0.9;
  Out.ar(out, main*amp);
}).writeDefFile("/app/supercollider/synthdefs");