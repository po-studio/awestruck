(
s.waitForBoot({
    play {
        var gain = 0.25;  // Master output gain

        GVerb.ar(
            // Apply GVerb to the summed and processed impulses
            GVerb.ar(
                // Generate impulses with staggered phases and frequencies, then scale
                Impulse.ar([8, 6, 4] / 100, [0, 0.2, 0.6]) * 8,
                [2, 3, 1.5],  // Reverb room sizes for inner GVerb
                90,  // Reverb damping for inner GVerb
                dryLevel: 0  // No dry signal mixed in the output of inner GVerb
            ).sum.sin.sum * 0.5,  // Sum impulses, apply sine transformation, then sum and scale
            62,  // Reverb room size for outer GVerb
            24  // Reverb damping for outer GVerb
        ).tanh * gain;  // Apply non-linear distortion and scale with gain
    }
});
)
