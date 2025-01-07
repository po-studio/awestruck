SynthDef.new("romero_1", { |out=0|
  var gain = 0.1;
  var sound = GVerb.ar(
    Blip.ar(
      (Duty.kr(
        1 / [1, 2, 4],
        0,
        Dseq([0, 3, 7, 12, 17] + 24, inf)
      ).midicps * [1, 4, 8]),
      LFNoise1.kr(1 / 4, 3, 4)
    ).sum * gain,
    200,
    8
  );
  Out.ar(out, sound);
}).writeDefFile("/app/supercollider/synthdefs");