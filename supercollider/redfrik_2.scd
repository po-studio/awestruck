(
s.waitForBoot({
    (
        play {
            var gain = 0.75;
            { CombL.ar(In.ar(8).tanh / 8, 1, 1, 8) ! 2 }.play;
            Pbind(
                \amp, 8,
                \dur, 1 / 4,
                \degree, Pseq(List.fib(32) % (List.fib(64) % 12), inf),
                \out, 8
            ).play;
        }
    );
});
)
