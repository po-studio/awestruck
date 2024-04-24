(
s.waitForBoot({
    (
        play {
            var gain = 0.25;
            GVerb.ar(
                {
                    |i|
                    Ringz.ar(
                        LPF.ar(
                            Impulse.ar(2 / (1.2 + i)),
                            6000
                        ),
                        1000 / (i + 1),
                        0.3
                    )
                }.dup(20).scramble.sum / 2,
                18,
                5
            ).tanh * gain;
        }
    );
});
)
