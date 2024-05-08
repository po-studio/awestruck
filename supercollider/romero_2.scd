s.options.numInputBusChannels = 0;
s.options.numOutputBusChannels = 2;
s.reboot;

s.waitForBoot {
  SynthDef.new("romero_3", { |out=0|
    var gain = 0.25;
    var d = Duty;

    var f = d.kr(
      1 / [1, 2, 3],
      0,
      Dseq([0, 3, 7, 12, 8] + 36, inf)
    );

    GVerb.ar(
      Blip.ar(
        f.midicps * [1, 2, 3],
        LFNoise1.kr(1, 8, 8)
      ).sum,
      99,
      4
    ) * gain;
    Out.ar(out, sound);
  }).writeDefFile(thisProcess.nowExecutingPath.dirname.withTrailingSlash ++ "synthdefs/");
};
