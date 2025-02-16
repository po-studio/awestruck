(
s.waitForBoot({
  Ndef('x', {
    var noise = LFNoise1.ar(0.1, 3);
    var osc = SinOsc.ar(65, Ndef('x').ar * noise, LFNoise1.ar(3, 6));
    var processed = osc.tanh;
    9.do {
      processed = AllpassL.ar(processed, 0.3, {0.2.rand + 0.1}!2, 5);
    };
    processed.tanh;
  }).play;
});
)
