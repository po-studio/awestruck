(
s.waitForBoot({
  play {
    var gain = 0.25;
    Resonz.ar(
      Crackle.ar ! 2,  
      Duty.kr(
        Dseq([1, 1, 4, 2, 2] / 8, inf),  
        0,  
        Dseq([99, Dwhite(99, 9000, 1)], inf)  
      ),
      TExpRand.kr(0.001, 1, Impulse.kr(8))  
    ) * 4 * gain;  
  }
});
)
