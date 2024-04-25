(
s.waitForBoot({
    play {
        var p = LFPulse;  // Low frequency pulse generator
        var gain = 0.5;  // Output gain

        var output = tanh(
            p.ar([50, 52]) *  // Audio rate pulse oscillators at 50 and 52 Hz
            p.kr([2, 1] / 4) +  // Control rate pulses for modulation
            mean({|n|  // Average of the complex expression across two iterations
                (
                    p.ar(n * 300 + 99 * p.kr(2 - n / [1, 5, 7], (0..2) / 10).sum + 200) *  // Audio rate pulses with complex frequency calculations
                    p.kr(n + 1 * 6, 0, 0.8).lag  // Control rate pulses with lag for smoothing
                )
            } ! 2) / 2  // Iterating twice, dividing the sum by 2 for averaging
        );

        output * gain;  // Scale the output by the gain
    }
});
)
