package main
import (
  "fmt"
  "image"
  "math/rand"
  "os"
  "github.com/foqerhk/runeverything/internal/desktop"
)
func main(){
  for _, mode := range []string{"solid","noise"} {
    enc,err:=desktop.NewEncoderBitrateCodec(15360,8640,5,250000,true)
    if err!=nil{panic(err)}
    img:=image.NewRGBA(image.Rect(0,0,15360,8640))
    if mode=="solid" {
      for i:=0;i<len(img.Pix);i+=4{img.Pix[i]=30;img.Pix[i+1]=60;img.Pix[i+2]=180;img.Pix[i+3]=255}
    } else {
      rng:=rand.New(rand.NewSource(1))
      for i:=0;i<len(img.Pix);i+=4{img.Pix[i]=byte(rng.Intn(256));img.Pix[i+1]=byte(rng.Intn(256));img.Pix[i+2]=byte(rng.Intn(256));img.Pix[i+3]=255}
    }
    b,err:=enc.Encode(desktop.Frame{Img:img}, true)
    if err!=nil{fmt.Println(mode,"fail",err);enc.Close();os.Exit(1)}
    fmt.Printf("%s IDR annexB=%d\n", mode, len(b))
    b2,_:=enc.Encode(desktop.Frame{Img:img}, false)
    fmt.Printf("%s P annexB=%d\n", mode, len(b2))
    enc.Close()
  }
}
