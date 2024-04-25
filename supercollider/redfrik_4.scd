(
s.waitForBoot({
    play {
        var gain = 0.25;  // Master output gain

        // Frequency control using a low-frequency parabolic oscillator
        var f = LFPar.ar(1 / 14).round * 20 + 80;  

        // Spatial distribution of sounds
        Splay.ar(
            // Generate a stereo field spread of low-frequency parabolic oscillators
            LFPar.ar({ |i| 
                [i + 1 * f, i * f + (i + 1 / 3)]  // Frequency modulation based on index
            }.dup(4)) >  // Duplicate the function to create four variations
            // Compare above signals to brown noise processed by a pulse wave
            BrownNoise.ar(
                Pulse.ar({ |i| i + 1 }.dup(4), 0.35)  // Pulse oscillator with increasing frequency
            ) / 3  // Scale down the noise
        ) * gain;  // Apply gain to the final output
    }
});
)
