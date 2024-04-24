(
s.waitForBoot({
    (
        var n = LFNoise1;
        var gain = 0.25;
        Ndef('x', {
            var a = SinOsc.ar(
                65,
                Ndef('x').ar * n.ar(0.1, 3),
                n.ar(3, 6)
            ).tanh;
            9.do {
                a = AllpassL.ar(
                    a,
                    0.3,
                    { 0.2.rand + 0.1 } ! 2,
                    5
                );
            };
            a.tanh * gain;
        }).play;
    );
});
)
