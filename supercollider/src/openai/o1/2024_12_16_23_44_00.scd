s.options.numInputBusChannels = 0;
s.options.numOutputBusChannels = 2;
s.reboot;

(
s.waitForBoot {

    SynthDef(\generative_composition, { |out=0|
        var sig, fb, pit, env, seq, res, drm;
        var shift1, shift2, lfoRate, ampLfo, dynImp, longFade;
        var baseText, slowTrig, baseFreq, sawCluster, ping;
        var rev;
        var tempo, kickTrig, snareTrig, hatTrig, kick, snare, hat, drumMix;

        //=====================================================================
        // CORE TEXTURAL BED
        //=====================================================================
        slowTrig = Dust.ar(0.05);
        baseFreq = 200;
        sawCluster = Array.fill(8, { |i|
            var freqVar = LFNoise2.kr(0.2).range(-10, 10);
            var freq = baseFreq + (i*baseFreq*0.05) + freqVar;
            var amp = 0.07 / (i+1);
            RLPF.ar(Saw.ar(freq, amp), freq*4, 0.4)
        }).sum;

        ping = Decay.ar(Impulse.ar(LFNoise2.kr(0.1).range(0.001,0.03), 0), 0.1, 0.2);
        baseText = sawCluster + ping;

        //=====================================================================
        // FEEDBACK & REVERB
        //=====================================================================
        rev = GVerb.ar(baseText*0.5, 60, 30, 0.5, 0.7, 0.5, 0.7, 0.5, 0.3);
        fb = LocalIn.ar(2);
        fb = HPF.ar(fb + rev, 10);

        //=====================================================================
        // PITCH SHIFTING
        //=====================================================================
        shift1 = LFNoise2.kr(0.01).range(0.45,0.55);
        shift2 = LFNoise2.kr(0.01).range(1.8,2.2);

        pit = PitchShift.ar(fb, 0.3, shift1, 0, 0.01, 0.9)
            + PitchShift.ar(fb, 0.3, shift2, 0, 0.01, 0.9);
        pit = tanh(pit * 1.5);
        LocalOut.ar(pit);

        //=====================================================================
        // STEREO MOTION & DYNAMICS
        //=====================================================================
        env = Amplitude.kr(baseText, 0.01, 1).lag(0.1);
        pit = Rotate2.ar(pit[0], pit[1], LFNoise1.kr(0.4));
        sig = baseText + (pit * 0.4 * env);

        //=====================================================================
        // RHYTHMIC SHAPING OF TEXTURE
        //=====================================================================
        lfoRate = 2 ** LFNoise2.kr(0.005).range(0,4).round;
        ampLfo = LFPulse.ar(lfoRate, [0, 0.5], 0.5)
            .range(LFNoise1.kr(0.01).range(0,0.9),1)
            .lag(0.01);

        sig = sig * ampLfo;

        dynImp = Impulse.ar(2 ** LFNoise0.kr(0.02).range(0,4).round, 0);
        sig = sig * Decay.ar(dynImp, 2, -1, 1);

        //=====================================================================
        // HIGH-FREQ TEXTURAL DETAILS
        //=====================================================================
        sig = RLPF.ar(sig, 6000, 0.7);
        sig = sig + (LFNoise2.ar(3,0.0021).thresh(0.0007).lag(0.5) * 0.7)
            - Decay.ar(Dust2.ar(0.3),0.2,0.002).lag(0.3);

        //=====================================================================
        // SEQUENCED FREQUENCIES FOR DRONE TEXTURES
        //=====================================================================
        seq = Dseq([50, 40, 60, 50, 80, 60], inf);
        res = Demand.kr(Impulse.kr(1/4,0), 0, seq);

        //=====================================================================
        // PERCUSSIVE TEXTURES (DRM)
        //=====================================================================
        drm = Ringz.ar(Impulse.ar([1.5,1]/2), 40, 0.1, 8).sin
            + Ringz.ar(Impulse.ar(0.75,0.25), 1200, 0.02, PinkNoise.ar(0.05)) * 0.5;

        drm = RLPF.ar(drm, res*40+300, 0.5) * 0.3;
        drm = drm + RLPF.ar(CombL.ar(drm.reverse, 0.3, 0.3, 2), LFNoise1.kr(0.3).range(500,1500), 0.4);
        drm = HPF.ar(drm,30);
        drm = DelayL.ar(drm,0.05,0.05) + Limiter.ar(drm*3,0.6)*0.7;

        //=====================================================================
        // DRUM PATTERN (MORE INTERESTING BEAT)
        //=====================================================================
        // Define a stable tempo: 2 Hz = 120 BPM quarter notes
        // Kick on quarter notes, Snare on beats 2 and 4, Hi-hats on 8th notes
        tempo = 2; // quarter notes
        kickTrig = Impulse.ar(tempo);
        snareTrig = Impulse.ar(tempo, 0.5); // offset by half a beat
        hatTrig = Impulse.ar(tempo*2); // eighth notes

        kick = Decay2.ar(kickTrig, 0.005, 0.05) * SinOsc.ar(50,0)*0.03;
        snare = Decay2.ar(snareTrig, 0.01, 0.1) * (WhiteNoise.ar(0.02).clip2(0.02) + SinOsc.ar(200,0)*0.01);
        hat = Decay2.ar(hatTrig, 0.001, 0.03) * HPF.ar(WhiteNoise.ar(0.01), 8000);

        // Mix the drum sounds together; keep them relatively dry and quiet
        drumMix = (kick + snare + hat)*0.5; // scale down overall drums

        //=====================================================================
        // MIXING & LONG-TERM EVOLUTION
        //=====================================================================
        longFade = Line.kr(0,0.4,180);  // fade in over 3 minutes
        sig = HPF.ar(sig,100) * longFade;

        // Add drm (textural percussion) at a lower level
    sig = sig + (tanh(drm*2)*0.3);
        // Add drum pattern
    // sig = sig + drumMix;

        sig = BPeakEQ.ar(sig,2200,1,-5);

        // Soft harmonic clouds
        sig = sig + (
            Ringz.ar(PinkNoise.ar(0.005!2), [1200,1600,2000],0.3).sum * Line.kr(0,0.3,240)
        );

        // Final limiting
        Out.ar(out, Limiter.ar(sig * 1.8, 1, 0.02));

    }).add;

    x = Synth(\generative_composition);
};
)
