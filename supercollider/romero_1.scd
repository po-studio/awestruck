(
s.waitForBoot({
  play {
    var gain = 0.1;  
    GVerb.ar(
      Blip.ar(
        (Duty.kr(
          1 / [1, 2, 4],  
          0,  
          Dseq(
            [0, 3, 7, 12, 17] + 24,  
            inf  
          )
        ).midicps * [1, 4, 8]),  
        LFNoise1.kr(1 / 4, 3, 4)  
      ).sum * gain,  
      200,  
      8     
    )
  }
});
)
