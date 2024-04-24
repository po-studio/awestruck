(
s.waitForBoot({
    (
        play {
            var gain = 0.75;
            var a = PinkNoise.ar(5e-4);
            GVerb.ar(
                ({ |i|
                    ar(
                        Ringz.ar(
                            a * LFNoise1.kr(0.2),
                            exprand(60, 8000),
                            3
                        )
                    )
                }.dup(40).sum),
                50,
                99
            ).tanh * gain;
        }
    );
});
)
