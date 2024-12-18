SynthDef.new("openai-o1-preview-2024_12_15_15_21_00", { |out=0, amp=0.5, dcy=3.0, bd=1, sd=1|
  var trigs, sig1, sig, fb, pit, env, seq, res, drm;
  
  // Generate rhythmic impulses
  trigs = { |i| Impulse.ar(0.08 / (i + 1), 0.4) }.dup(8);
  
  // Layered filtered noise with decaying envelopes
  sig1 = { |i|
    RLPF.ar(
      WhiteNoise.ar(Decay.ar(trigs[i], 2)),
      200 * i + 400,
      0.3
    )
  }.dup(8).sum;
  
  // Reverb processing
  sig = GVerb.ar(sig1 * 0.6, 50, 25, 0.15, spread: 20);
  
  // Feedback loop with high-pass filtering
  fb = HPF.ar(LocalIn.ar(2) + sig, 80);
  
  // Pitch shifting with detune
  pit = PitchShift.ar(fb, 0.35, 0.7, 0, 0.01, 1.0) + 
  PitchShift.ar(fb, 0.65, 1.4, 0, 0.01, 0.95);
  pit = tanh(pit * 1.4);
  
  LocalOut.ar(pit);
  
  // Modulated pulse for stereo rotation
  pit = Rotate2.ar(pit[0], pit[1], LFNoise1.kr(2.0) * LFNoise1.kr(1.5));
  
  // Amplitude envelope follower
  env = Amplitude.kr(sig1, 0.01, 1, -1, 1);
  
  // Combine the signals with dynamic modulation
  sig = sig + (pit * 0.5 * env);
  
  // Additional decay and filtering
  sig = sig * Decay.ar(
    Impulse.ar(2 ** LFNoise0.kr(0.5).range(0, 3).round),
    2,
    -1,
    1
  );
  
  sig = RLPF.ar(sig, 5000, 0.6);
  
  // Drum elements
  drm = Ringz.ar(Impulse.ar([1, 1.5] / 2, [0, 0.75]) * bd, 50, 0.2, 7)
  + Ringz.ar(Impulse.ar(1.0, 0.5) * sd, 700, 0.03) * LFSaw.ar(3).range(0.7, 0.1);
  
  drm = RLPF.ar(drm, 3000, 0.5);
  drm = HPF.ar(drm, 50);
  
  // Combine everything and add limiter
  sig = HPF.ar(sig, 120) * Line.kr(0, 0.3, 150);
  sig = sig + drm;
  
  sig = Limiter.ar(sig * 1.5, 0.9);
  
  Out.ar(out, sig * amp);
}).writeDefFile("/app/supercollider/synthdefs");
