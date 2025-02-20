(
// Create a SynthDef instead of using play
SynthDef("abio_derivation_7", { |out=0, dcy=4, bd=1, sd=1|

    //////////////////////////////////////////////////////////////////////////
    // ALL VARIABLES AT THE TOP
    //////////////////////////////////////////////////////////////////////////

    var time, silenceGate, buildupGate, dropGate,
    chordArray, chordSeq, chordIndex, chord,
    buildPad, dropPad, localIn, feedback, pit,
    leadGate, leadNotes, leadFreq, leadEnv, lead,
    // multiple drum layers for the "progressive" build
    t0, t1, t2, t3,
    drumsPhase1, drumsPhase2, drumsPhase3, drumsPhase4, buildDrums,
    dropDrums, subBass, hiHat, drums,
    sparkleFreq, sparkleEnv, sparkles,
    clap, fillTrig, fillRing, fill,
    mix, bassSeq, bassFreq, bassEnv;

    //////////////////////////////////////////////////////////////////////////
    // 1) TIMING & GATES
    //////////////////////////////////////////////////////////////////////////
    time        = Line.kr(0, 999, 999);       // clock in seconds
    buildupGate = (time < 32).lag(0.02);      // 1 until t=32
    dropGate    = (time >= 32).lag(0.02);     // 1 after t=32

    // Silence for dramatic pause from t=30.5..32
    silenceGate = 1 - ((time >= 30.5) * (time < 32));

    // Further gates for progressive layering of the beat in the buildup:
    // t0 = 0-8s, t1 = 8-16s, t2 = 16-24s, t3 = 24-30.5s
    t0 = (time < 8).lag(0.02);
    t1 = ((time >= 8)    * (time < 16)).lag(0.02);
    t2 = ((time >= 16)   * (time < 24)).lag(0.02);
    t3 = ((time >= 24)   * (time < 30.5)).lag(0.02);

    //////////////////////////////////////////////////////////////////////////
    // 2) CHORD PROGRESSION
    //////////////////////////////////////////////////////////////////////////
    chordArray = [
        [48, 51, 55, 60], // C minor
        [53, 56, 60, 65], // F minor
        [56, 60, 63, 68], // Ab major
        [51, 55, 58, 63], // Eb major
        [55, 58, 62, 67], // G minor
        [58, 62, 65, 70], // Bb major
        [53, 56, 60, 65], // F minor
        [55, 58, 62, 67]  // G minor
    ];
    // 1 chord every 2 seconds
    chordSeq   = Dseq(Array.series(chordArray.size, 0, 1), inf);
    chordIndex = Demand.kr(Impulse.kr(0.5), 0, chordSeq);
    chord      = Select.kr(chordIndex, chordArray).midicps; // array of freq

    //////////////////////////////////////////////////////////////////////////
    // 3) BUILD-UP PAD
    //////////////////////////////////////////////////////////////////////////
    buildPad = Mix.fill(4, { |i|
        var exc = Impulse.ar(0.125/(i+1));
        RLPF.ar(
            Saw.ar(chord[i], Decay.ar(exc, 2)),
            600 + (i*400),
            0.3
        )
    });
    buildPad = GVerb.ar(buildPad * 0.4, 50, 30, 0.2, spread: 30);
    // Filter sweep 800→5000 over 32s
    buildPad = RLPF.ar(
        buildPad,
        Line.kr(800, 5000, 32),
        0.5
    ) * buildupGate;

    //////////////////////////////////////////////////////////////////////////
    // 4) DROP PAD
    //////////////////////////////////////////////////////////////////////////
    dropPad = Mix.fill(4, { |i|
        var exc = Impulse.ar(0.25/(i+1));
        RLPF.ar(
            Pulse.ar(chord[i], width:0.4, mul: Decay.ar(exc,1.5)),
            800 + (i*600),
            0.4
        )
    });
    dropPad = GVerb.ar(dropPad * 0.5, 40, 30, 0.3, spread:25);
    dropPad = RLPF.ar(dropPad, 7000, 0.6) * dropGate;

    //////////////////////////////////////////////////////////////////////////
    // 5) FEEDBACK & LIGHT PITCH SHIFT
    //////////////////////////////////////////////////////////////////////////
    localIn  = LocalIn.ar(2);
    feedback = HPF.ar(localIn + (buildPad + dropPad), 60);

    pit = PitchShift.ar(feedback, 0.5, 1.02, 0, 0.01, 0.98);
    pit = tanh(pit * 1.2);
    LocalOut.ar(pit);

    mix = (buildPad + dropPad) + (0.3 * pit);

    //////////////////////////////////////////////////////////////////////////
    // 6) LEAD MELODY
    //////////////////////////////////////////////////////////////////////////
    leadGate  = (time > 8).lag(0.01);

    leadNotes = Dshuf(chordArray.collect { |c| c.choose }, inf);
    leadFreq  = Demand.kr(Impulse.kr(4), 0, leadNotes).midicps;

    leadEnv   = Decay2.ar(Impulse.ar(4), 0.01, 0.3);
    lead      = SinOsc.ar(leadFreq, 0, leadEnv * 0.4);
    lead      = CombC.ar(lead, 0.3, 0.3, 2, mul: 0.6);
    lead      = RLPF.ar(
        lead,
        EnvGen.kr(Env([2000,8000],[24]), (time >=32)),
        0.7
    ) * leadGate;

    mix = mix + lead;

    //////////////////////////////////////////////////////////////////////////
    // 7) PROGRESSIVE DRUMS DURING BUILDUP
    //////////////////////////////////////////////////////////////////////////
    // PHASE 1 (0-8s): Kick every beat
    drumsPhase1 = Ringz.ar(
        Impulse.ar(1) * DC.ar(bd) * t0, // 1 beat/s
        50, 0.3, 4
    ).tanh;

    // PHASE 2 (8-16s): Add snare on 2 & 4
    drumsPhase2 = Ringz.ar(
        Impulse.ar(2) * DC.ar(sd) * t1, // 2 beats/s => hits on 2 & 4 if BPM ~ 60
        1500, 0.02, 3
    ).tanh;

    // PHASE 3 (16-24s): Add hats at 4 beats/s
    drumsPhase3 = BPF.ar(WhiteNoise.ar(0.1), 8000, 0.005)
        * Decay.ar(Impulse.ar(4) * t2, 0.05) * 0.6;

    // PHASE 4 (24-30.5s): Add random "ghost" hits
    // triggered at 3 Hz with ringz-based resonance
    drumsPhase4 = Ringz.ar(
        Impulse.ar(3)*t3,
        LFNoise0.kr(0.5).exprange(200,1800),
        0.04,
        2
    ).tanh * 0.4;

    // Sum them, filter & gate for buildup
    buildDrums = (drumsPhase1 + drumsPhase2 + drumsPhase3 + drumsPhase4) * buildupGate;
    buildDrums = RLPF.ar(buildDrums, 2500, 0.5);

    //////////////////////////////////////////////////////////////////////////
    // 8) DRUMS - DROP (heavier)
    //////////////////////////////////////////////////////////////////////////
    // Main four-on-the-floor
    dropDrums = Ringz.ar(Impulse.ar(1) * DC.ar(bd), 45, 0.2, 6).tanh * 1.8
              + Ringz.ar(Impulse.ar(2) * DC.ar(sd), 1800, 0.02, 2).tanh;
    // Add comb reverb for thickness
    dropDrums = dropDrums + RLPF.ar(CombL.ar(dropDrums,0.25,0.25,dcy), 1200, 0.4);

    // Sub-bass line (quarter note pattern)
    bassSeq  = Dseq([36, 36, 38, 36, 34, 34, 36, 31], inf);
    bassFreq = Demand.kr(Impulse.kr(4)*dropGate, 0, bassSeq).midicps;
    bassEnv  = Decay2.ar(Impulse.ar(4)*dropGate, 0.01, 0.2);

    subBass = tanh(SinOsc.ar(bassFreq, 0, bassEnv*0.7)*3);
    subBass = LPF.ar(subBass, 200);
    subBass = HPF.ar(subBass, 30);
    subBass = subBass * dropGate;

    dropDrums = dropDrums * dropGate + subBass;

    //////////////////////////////////////////////////////////////////////////
    // 9) DRUM EXTRAS ON DROP (Clap, hats, random fill)
    //////////////////////////////////////////////////////////////////////////
    // Clap on 2 & 4
    clap = Ringz.ar(
        Impulse.ar(2)*DC.ar(dropGate),
        800, 0.02, 6
    ).tanh * 1.2;

    // 16th-note hi-hats
    hiHat = BPF.ar(WhiteNoise.ar(0.1), 8000, 0.005)
          * Decay.ar(Impulse.ar(16)*dropGate, 0.04) * 0.4;

    // Random fill once per measure (Impulse.ar(1) if BPM=60)
    fillTrig = Impulse.ar(1)*dropGate;
    fillRing = Demand.kr(DC.kr(fillTrig), 0, Drand([220,330,440,550,660], inf));
    fill     = Ringz.ar(fillTrig, fillRing, 0.1, 3).tanh * 0.5;

    dropDrums = dropDrums + clap + hiHat + fill;

    //////////////////////////////////////////////////////////////////////////
    // COMBINE BUILDUP & DROP DRUMS
    //////////////////////////////////////////////////////////////////////////
    drums = buildDrums + dropDrums;
    drums = HPF.ar(drums, 30);
    drums = Limiter.ar(drums*5, 0.7);

    mix = mix + drums;

    //////////////////////////////////////////////////////////////////////////
    // 10) SOFTER, AIRY "SPARKLES"
    //////////////////////////////////////////////////////////////////////////
    sparkleFreq = Demand.kr(Impulse.kr(2), 0, Dshuf([72,74,76,77,79,81,83,84], inf)).midicps;
    sparkleEnv  = Decay2.ar(Impulse.ar(2)*dropGate, 0.01, 0.4);
    sparkles    = SinOsc.ar(sparkleFreq * 1.001, LFNoise1.kr(0.5).range(0, 2pi), sparkleEnv * 0.4)
                + SinOsc.ar(sparkleFreq * 0.999, 0, sparkleEnv * 0.4);
    sparkles    = RLPF.ar(sparkles, 9000, 0.2);
    sparkles    = FreeVerb.ar(sparkles, 0.5, 0.7, 0.3);
    sparkles    = sparkles * dropGate * 0.5;

    mix = mix + sparkles;

    //////////////////////////////////////////////////////////////////////////
    // 11) STRATEGIC SILENCE BEFORE DROP
    //////////////////////////////////////////////////////////////////////////
    mix = mix * silenceGate;

    //////////////////////////////////////////////////////////////////////////
    // 12) FINAL SHAPING
    //////////////////////////////////////////////////////////////////////////
    // Overall fade-in from 0→0.5 over ~30s
    mix = mix * Line.kr(0.0, 0.5, 30);

    // Subtle notch around 2.5k + slight shimmer
    mix = BPeakEQ.ar(mix, 2500, 1, -4)
        + (Ringz.ar(PinkNoise.ar(0.001!2), [1500,2000], 0.3).sum * 0.1);

    // Final output
    Out.ar(out, Limiter.ar(mix, 1, 0.02));

}).add;
)
