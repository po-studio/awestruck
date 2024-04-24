(
s.waitForBoot({
    (
        play {
            var gain = 0.25;
            var f = LFPar.ar(1 / 14).round * 20 + 80;
            Splay.ar(
                LFPar.ar({ |i| [i + 1 * f, i * f + (i + 1 / 3)] }.dup(4)) >
                BrownNoise.ar(
                    Pulse.ar({ |i| i + 1 }.dup(4), 0.35)
                ) / 3
            ) * gain;
        }
    );
});
)
