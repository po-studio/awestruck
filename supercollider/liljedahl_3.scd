(
s.waitForBoot({
    play {
        var gain = 0.25;  // Master output gain
        
        // Processed output using GVerb for reverb
        GVerb.ar(
            // Sum of scrambled array of 20 duplicate filtered impulse responses
            {
                |i|  // Function with index i for modulation
                Ringz.ar(  // Ring modulation
                    LPF.ar(  // Low-pass filter
                        Impulse.ar(2 / (1.2 + i)),  // Impulse generator with decreasing frequency
                        6000  // Filter cutoff frequency
                    ),
                    1000 / (i + 1),  // Ring frequency decreases with index
                    0.3  // Decay time for ring modulation
                )
            }.dup(20).scramble.sum / 2,  // Duplicate the function 20 times, scramble order, sum them, and halve the result
            18,  // Reverb room size
            5    // Reverb damping
        ).tanh * gain;  // Apply non-linear distortion with tanh and scale with gain
    }
});
)
