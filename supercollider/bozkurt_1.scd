(
s.waitForBoot({
    (
        play {
            var gain = 0.25;
            LocalOut.ar(
                x = DelayC.ar(
                    LPF.ar(
                        LFNoise0.ar(8) ** 2 + LocalIn.ar(2).tanh.round(0.05),
                        6000
                    ),
                    1,
                    LFNoise0.ar(8 ! 2).range(1e-4, 0.02)
                )
            ).tanh * gain;
        }
    );
});
)
