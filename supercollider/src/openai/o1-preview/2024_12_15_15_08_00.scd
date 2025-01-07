SynthDef("openai-o1-preview-2024_12_15_15_08_00", {
  var bpm, beat, chordTrig, scaleU, scaleSize, chordRootSeq, chordRoot, chordIntervalsU;
  var chordDeg0, chordDeg1, chordDeg2, iChordDeg0, iChordDeg1, iChordDeg2;
  var noteDeg0, noteDeg1, noteDeg2, chordF0, chordF1, chordF2;
  var chordEnv, pad;
  var bdTrig, bdEnv, bd, halfBeat, sdTrig, snEnv, sdNoise, sd;
  var noteTrig, noteSeq, melody, noteEnv, melDeg, iMel, noteFreq, melSig;
  var main, fb, pit, rev, diff, final, baseFreq;
  
  bpm = 60;
  beat = Impulse.kr(bpm/60); // one beat per second at 60 BPM
  
  // Base frequency (A2 ~110 Hz)
  baseFreq = DC.kr(110);
  
  // Scale in semitones relative to A: A natural minor
  scaleU = [DC.kr(0), DC.kr(2), DC.kr(3), DC.kr(5), DC.kr(7), DC.kr(8), DC.kr(10)];
  scaleSize = 7;
  
  // Chord progression changes every 16 beats
  chordTrig = PulseDivider.kr(beat, 16, 0);
  // Integer steps for chord roots
  chordRootSeq = Dseq([0,5,10,7], inf);
  chordRoot = Demand.kr(chordTrig, 0, chordRootSeq); // integers from the sequence
  
  // Triad intervals (in scale steps)
  chordIntervalsU = [DC.kr(0), DC.kr(2), DC.kr(4)];
  
  // Compute chord degrees (add root + intervals)
  chordDeg0 = chordRoot + chordIntervalsU[0];
  chordDeg1 = chordRoot + chordIntervalsU[1];
  chordDeg2 = chordRoot + chordIntervalsU[2];
  
  // Ensure indices are integers by flooring (they should already be integers, but just to be safe)
  iChordDeg0 = floor(chordDeg0 % scaleSize);
  iChordDeg1 = floor(chordDeg1 % scaleSize);
  iChordDeg2 = floor(chordDeg2 % scaleSize);
  
  noteDeg0 = Select.kr(iChordDeg0, scaleU);
  noteDeg1 = Select.kr(iChordDeg1, scaleU);
  noteDeg2 = Select.kr(iChordDeg2, scaleU);
  
  // Convert scale steps (semitones) to frequency
  chordF0 = baseFreq * (2 ** (noteDeg0/12));
  chordF1 = baseFreq * (2 ** (noteDeg1/12));
  chordF2 = baseFreq * (2 ** (noteDeg2/12));
  
  // Pad envelope
  chordEnv = EnvGen.kr(Env.asr(4,1,8), chordTrig);
  pad = Mix([
    RLPF.ar(LFSaw.ar(chordF0, Rand(0,1))*0.05, LFNoise1.kr(0.2).exprange(500,3000),0.4),
    RLPF.ar(LFSaw.ar(chordF1, Rand(0,1))*0.05, LFNoise1.kr(0.2).exprange(500,3000),0.4),
    RLPF.ar(LFSaw.ar(chordF2, Rand(0,1))*0.05, LFNoise1.kr(0.2).exprange(500,3000),0.4)
  ]) * chordEnv * 0.6;
  
  // Bass Drum every 4 beats
  bdTrig = PulseDivider.kr(beat,4,0);
  bdEnv = EnvGen.kr(Env.perc(0.001,0.2,1,-4), bdTrig);
  bd = (SinOsc.ar(50)*0.8 + Ringz.ar(Impulse.ar(bdTrig),60,0.15,0.4)) * bdEnv;
  
  // Snare on beats 2 and 4
  halfBeat = PulseDivider.kr(beat,2,1);
  sdTrig = halfBeat * (1 - bdTrig);
  snEnv = EnvGen.kr(Env.perc(0.001,0.15,1,-4), sdTrig);
  sdNoise = WhiteNoise.ar(0.2)*snEnv;
  sd = RLPF.ar(sdNoise,2000,0.5)*0.4 + Ringz.ar(sdNoise,3000,0.07,0.3);
  
  // Melody every 4 beats (on beat 2)
  noteTrig = PulseDivider.kr(beat,4,2);
  noteSeq = Dseq([0,3,5,7,10,7,5,3],inf); // integers
  melody = Demand.kr(noteTrig,0,noteSeq); // integers from sequence
  noteEnv = EnvGen.kr(Env.perc(0.01,1,1,-4), noteTrig);
  
  iMel = floor(melody % scaleSize);
  melDeg = Select.kr(iMel, scaleU);
  noteFreq = baseFreq * (2 ** (melDeg/12));
  melSig = SinOsc.ar(noteFreq,0,0.15)*noteEnv;
  melSig = RLPF.ar(melSig,LFNoise1.kr(0.1).exprange(800,4000),0.4);
  
  // Combine main elements
  main = pad + bd + sd + melSig;
  
  // Local feedback chain with shimmer
  fb = LocalIn.ar(2);
  fb = HPF.ar(fb,80);
  pit = PitchShift.ar(fb, 0.5, 1.2,0,0.01,0.7)
  + PitchShift.ar(fb, 0.5,0.9,0,0.01,0.95);
  pit = tanh(pit * 1.3);
  LocalOut.ar(pit);
  main = main + pit*0.3;
  
  // Subtle resonance using a comb filter on chord root frequency
  main = main + CombL.ar(main,0.25,1/(chordF0+100),2)*0.1;
  
  // Reverb and diffusion
  rev = GVerb.ar(main,50,30,0.2,0.5,0.7,0.5,0.3,0.9);
  diff = AllpassC.ar(rev,0.05,0.03,1) + AllpassC.ar(rev,0.05,0.02,1);
  final = rev*0.7 + diff*0.3;
  
  final = HPF.ar(final,40);
  final = LPF.ar(final,8000);
  final = Limiter.ar(final,1,0.01);
  
  Out.ar(0, final*0.9);
}).writeDefFile("/app/supercollider/synthdefs");