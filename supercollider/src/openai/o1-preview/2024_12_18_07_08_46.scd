SynthDef.new("openai-o1-preview-2024_12_18_07_08_46", { |out=0, amp=0.5|
  var sound, scale, baseFreq, freqs, degrees,
  degreeSeq1, degreeSeq2, degreeSeq3, degreeSeqMel, degreeSeqRhythmic,
  freq1, freq2, freq3, freqMel, freqRhythmic,
  osc1, osc2, osc3, melOsc, rhythmicElement,
  lfo1, lfo2, lfo3, lfoFilter, lfoPan1, lfoPan2, lfoAmp1, lfoAmp2,
  melEnv, rhythmicEnv, noiseEnv, noise, filteredNoise,
  filteredSound, chorusDelay, filterFreq, melFilterFreq,
  roomSize, delayTime, feedback, delayLine,
  timbreMod, modIndex, modOsc1, modOsc2,
  ampEnv, reverbedSound, oscBlend;
  
  // Scale and base frequency
  scale = [0, 2, 3, 5, 7, 8, 10];
  baseFreq = 220;
  
  // LFOs for modulation
  lfo1 = LFNoise1.kr(0.05).range(-5, 5);
  lfo2 = LFNoise1.kr(0.03).range(-5, 5);
  lfo3 = LFNoise1.kr(0.07).range(-5, 5);
  lfoFilter = LFNoise1.kr(0.02).exprange(200, 2000);
  lfoPan1 = LFNoise1.kr(0.1).range(-1, 1);
  lfoPan2 = LFNoise1.kr(0.15).range(-1, 1);
  lfoAmp1 = LFNoise1.kr(0.08).range(0.2, 0.8);
  lfoAmp2 = LFNoise1.kr(0.12).range(0.2, 0.8);
  timbreMod = SinOsc.kr(0.05).range(0.5, 1);
  modIndex = LFNoise1.kr(0.04).range(1, 5);
  
  // Degree sequences for oscillators
  degreeSeq1 = Demand.kr(Impulse.kr(0.1), 0, Dshuf(scale, inf));
  degreeSeq2 = Demand.kr(Impulse.kr(0.08), 0, Dshuf(scale, inf));
  degreeSeq3 = Demand.kr(Impulse.kr(0.12), 0, Dshuf(scale, inf));
  degreeSeqMel = Demand.kr(Impulse.kr(0.05), 0, Dseq(scale, inf));
  degreeSeqRhythmic = Demand.kr(Impulse.kr(1), 0, Dshuf(scale, inf));
  
  // Calculate frequencies from degrees
  freq1 = baseFreq * (2 ** (degreeSeq1 / 12)) + lfo1;
  freq2 = baseFreq * (2 ** (degreeSeq2 / 12)) + lfo2;
  freq3 = baseFreq * (2 ** (degreeSeq3 / 12)) + lfo3;
  freqMel = baseFreq * (2 ** (degreeSeqMel / 12));
  freqRhythmic = baseFreq * (2 ** (degreeSeqRhythmic / 12));
  
  modOsc1 = SinOsc.kr(freq1 * modIndex);
  modOsc2 = SinOsc.kr(freq2 * modIndex);
  
  // Main oscillators
  osc1 = SinOsc.ar(freq1 + modOsc1, 0, lfoAmp1) * timbreMod;
  osc1 = Pan2.ar(osc1, lfoPan1);
  
  osc2 = Saw.ar(freq2 + modOsc2, lfoAmp2) * (1 - timbreMod);
  osc2 = Pan2.ar(osc2, lfoPan2);
  
  oscBlend = Mix.new([osc1, osc2]);
  
  // Apply filters
  filterFreq = lfoFilter;
  filteredSound = RLPF.ar(oscBlend, filterFreq, 0.8);
  chorusDelay = DelayC.ar(filteredSound, 0.03, LFNoise1.kr(0.5).range(0.01, 0.03));
  filteredSound = filteredSound + (chorusDelay * 0.5);
  
  // Third oscillator for extra layer
  osc3 = Pulse.ar(freq3, 0.5, 0.1) * SinOsc.kr(0.03).range(0.3, 0.7);
  osc3 = Pan2.ar(osc3, LFNoise1.kr(0.05).range(-1, 1));
  filteredSound = filteredSound + osc3;
  
  // Melodic element
  melEnv = EnvGen.kr(Env.perc(0.5, 2, 1, -4), Impulse.kr(0.02));
  melOsc = SinOsc.ar(freqMel, 0, melEnv * 0.2);
  melFilterFreq = LFNoise1.kr(0.05).exprange(500, 4000);
  melOsc = BPF.ar(melOsc, melFilterFreq, 0.2) * (1 - timbreMod);
  
  // Noise texture
  noiseEnv = EnvGen.kr(Env.perc(5, 10, 1, -4));
  noise = PinkNoise.ar(noiseEnv * 0.05);
  filteredNoise = LPF.ar(noise, LFNoise1.kr(0.1).exprange(300, 2000));
  
  // Rhythmic pulses
  rhythmicEnv = EnvGen.kr(Env.perc(0.005, 0.1, 0.5, -4), Impulse.kr(1));
  rhythmicElement = SinOsc.ar(freqRhythmic, 0, rhythmicEnv * 0.2);
  rhythmicElement = Pan2.ar(rhythmicElement, LFNoise1.kr(0.5).range(-1, 1));
  
  // Envelope control
  ampEnv = EnvGen.kr(Env.asr(5, 1, 5), gate: 1, doneAction: 2);
  
  // Reverb and delay
  roomSize = SinOsc.kr(0.01).range(0.6, 0.9);
  reverbedSound = FreeVerb.ar(filteredSound + melOsc, mix: 0.3, room: roomSize);
  delayTime = LFNoise1.kr(0.1).range(0.1, 0.3);
  feedback = 0.3;
  delayLine = CombL.ar(reverbedSound, 0.5, delayTime, feedback);
  
  sound = (delayLine + rhythmicElement + filteredNoise) * ampEnv;
  sound = Limiter.ar(sound, 0.5);
  Out.ar(out, sound * amp);
}).writeDefFile("/app/supercollider/synthdefs");