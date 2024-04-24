(
s.waitForBoot({
    (
        var x = Ndef(\x, Pbind(
            \freq, Pseq(a = (3..5); a * .x a * .x[4, 8], 8)
        ));
        Ndef(\, {
            Limiter.ar(
                GVerb.ar(
                    PitchShift.ar(
                        Ndef.ar(\, 1, 2, 0, 0.1),
                        20,
                        20
                    ) / 4 + x.ar
                )
            );
        }).play;
    );
});
)
