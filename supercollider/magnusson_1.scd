(
s.waitForBoot({	
	(
        play {
            {
                var x = LFNoise0.ar(1) > 0;
                var freq = Spring.ar(x, 4, 3e-05) * (70.rand + 190) + LFNoise2.ar(1).range(90, 120);
                var env = EnvGen.kr(Env.perc(0.001, 5), x);
                var gain = 0.5;
                SinOsc.ar(freq) * env * gain;
            } ! 2
        }
	)
});
);