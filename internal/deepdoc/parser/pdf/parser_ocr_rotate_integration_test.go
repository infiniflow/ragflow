//go:build cgo && integration

package pdf

import (
	"bytes"
	"encoding/base64"
	"image"
	"strings"
	"testing"

	_ "image/jpeg"

	"ragflow/internal/deepdoc/parser/pdf/util"
)

// layer2CropB64 is a vertical (tall, h/w ~= 4.5) "RAGFlow" text crop, embedded
// as a JPEG so the test stays self-contained without committing a binary into
// the gitignored testdata/ directory. The crop is unreadable at 0 deg but reads
// cleanly once rotated upright (CW90), which is exactly what layer-2 must pick.
const layer2CropB64 = "/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/2wBDAQkJCQwLDBgNDRgyIRwhMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjL/wAARCAEEADkDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwD3+iiigAorz34m/EuT4dXGjH+zFvob7zvMHm+WybNmMHBH8Z/Ks/Rfj74M1Pal5Jd6ZKeD9ph3Jn2ZM/mQKAPUqKztL17SNci8zStTs71cZP2eZXx9QDx+NaNABRRRQAUUUUAcB8T/AIZj4iwadt1T7DNYebszD5ivv2ZzyMfcH514frHwD8a6bua0itNSjHObaYK2P919v5DNe2fFP4lyfDuDTRBpiXs1/wCbtLzFFj2bOoAOc7/UdK8XufjH8RvFNwbXSAYWb/ljploXfH1O5vyxQBwGoaHr3hu4Vr/Tr/TpVPyvJE0fP+y3f8K9N+DXjzxTe/EDStEvNburrTrgSiSK4bzD8sTsMM2WGCo6Gs+H4V/E7xfKs+rC4UE5Euq3ZJH/AAHLMPyr0r4efA+Xwh4ks9fvtbS4ubYPtt4ISEyyMhyxOTwx7CgD2OiiigAooooA4L4mfDSL4iwaeDqb2M1j5vlkQiRW37c5GR/cHfvXjGofAfxzok32nSLi2vGTlHtbgwyj/vrGPwJr1/4rfEu4+HkGmC102K8mv/N2tLKVWPZs6gD5s7/UdK8L1j44+OdW3LHqMWnxN/BZQhf/AB5ssPzoAs/8Jt8V/AxVdSl1NIQcY1KDzUb2EjDJ/Bq9E+G/xu1LxZ4psfD+p6RapLdCTFzbyMoXbGz/AHDnOduOo618/wAlxrvie+Akl1HVbtugZnnc/Tqa9Y+EHw28WaX480vXtR0iSzsLcSl2uGVX+aJ1GEzu6sOooA+lKKKKACiiigDg/iN8NIviJcaQbjU3s4bDztyxxB2k37OhJwMbPQ9aq6L8D/A+kbWk0+TUZV/jvZS//jowv5itf4j+OrfwF4aa/aNZr2ZvKtICeHfGcn/ZA5P4DvXyfr/jjxL4munn1TWLqUMciFZCkS+wQcD+dAH2tY6dY6ZbiCws7e0hHSOCJY1/ICrNfCGn69q+kzrPp2p3lrIpyGhmZf5HmvpH4OfFe48XM+ha4yHVoo/MinVQouEHXIHAYdeOo+hoA9eooooAKKKKAPmr9pK8kfxbpFiSfKhsTMo/2nkYH/0AV3vwu+Fvhi28HaZqmoaZbajf31uly8l0gkVA43BVU8DAI5xnOfoOY/aR8PzudI8QxIWhRWtJ2H8HO5PwOXH5etcp4F+OOreEdHh0i70+PU7KAYgzKYpI1/u7sEEDtx+OMUAe+6z8MvBut2UltP4fsYCwwJrSFYZEPqGUDp75HtXzL4Pil8M/GnTbGOXe9rrH2IyAfeBkMTH8QTXd6z+0lf3NlJDpGgxWc7LgTz3HnbPcLtAz9SR7Vx/we0W78SfFCxun3yJZyG+uZm55HK5PqXI/X0oA+vKKKKACiivFfHnxwv8Awf401DQYdFtrmO18vErzMpbdGr9AP9rFAHsGpabZ6xp0+n6hbpcWk67JYnHDD/PftXhWv/s3F7p5fD+tokLHIgvUOU/4GvX/AL5/OqP/AA0rqn/Qu2f/AH/b/Cj/AIaV1T/oXbP/AL/t/hQAzTv2bNYedf7T12xhhzybZHlY/wDfQWvb/B/gvR/BGk/YNJhYbyGmnkOZJm9WP8gOBXif/DSuqf8AQu2f/f8Ab/Cuj8B/HC/8YeNNP0GbRba2juvMzKkzMV2xs/Qj/ZxQB7VRRRQAV5B42+Bg8Y+L77Xv+EiNp9q8v9x9i8zbtjVPveYM5256d69fryD4q/FvV/AXii20uwsLG4ilsluC84fcGLuuOGHHyCgDn/8AhmUf9Dcf/Bd/9to/4ZlH/Q3H/wAF3/22vbPDmpS6z4X0nVJkRJb2yhuHRM7VZ0DEDPbmvNv+F5Rf8LA/4RT+wH3f2p/Zv2n7WMZ83y9+3Z+OM/jQBzn/AAzKP+huP/gu/wDttdB4J+Bg8HeL7HXv+EiN39l8z9x9i8vdujZPveYcY3Z6dq9fooAKKKKACvmD9o7/AJKHp/8A2Co//RstfT9fMH7R3/JQ9P8A+wVH/wCjZaAPf/An/JPPDX/YKtf/AEUtfMH/ADcL/wBzX/7d1Ppnxw8Y6TpVnptrJYi3tIEgi3W+TtRQoyc8nArjP+Egvv8AhK/+EkzH/aH277fnb8vm79/T03dqAPuuivlL/hoDxx/z00//AMBv/r11fw1+MHirxT8QNL0bUnszaXPm+YI4NrfLE7DBz6qKAPoKiiigArzf4gfCKz8f69Bqtxq09o8VqtsI44gwIDM2ck/7f6V6RXK+MfiFoPgY2i6zLMHutxjWGPecLjJPPHUUAebf8M1aZ/0Md3/4Dr/jR/wzVpn/AEMd3/4Dr/jXrXhfxNY+LtFTV9NS4FpI7IjTx7C204JA9M5H4GuL+Ivxfj+H/iC30p9Ea+M1qtz5gufLxl3XGNp/uZz70Acz/wAM1aZ/0Md3/wCA6/41u+DvgfY+D/FVlr0OtXNzJa78RPCqhtyMnUH/AGs/hXNf8NMQf9CrJ/4Hj/43XZfDn4u2nj/VrrTf7MbT7iGHzkDTiTzFzhv4RjGR+ftQB6TRRRQAV8qftA6i158S2tS3yWVpFEB6Fsuf/Qx+VfVdfIvxyheL4taq7A4ljgdfp5SL/NTQB9R+FtJj0LwppWlxqALa1jjOO7bRuP4nJ/Gvnf8AaO/5KHp//YKj/wDRstfRvh/VoNe8Pafqts4aK6gSQEHoSOR9Qcg/SsXxP8N/C/jDUo9Q1uxknuY4RArLO6YQFmAwpA6saAPCNH/Z/wBd1nQ9P1SHV9OjivbaO4RHD7lDqGAOB15rB+GUtx4a+MemWsx2yJePYzAdCW3Rkf8AfWD+FfWtpbWeiaPBaQ4hsrG3WNN7ZCRouBkn0A6mvkPQLsa38brG/tgdl1r63KjH8Jn3/wAqAPsiiiigArwX9onwhNcR2Xiq0iLrAn2a82j7q5JR/pkkE+6171Uc8EN1byW9xEksMqlHjdQVZTwQQeooA+PPAvxT1/wIrW1oY7vTnbc1pcZ2g9ypHKn9PavS0/aYh8rL+FX8zHQXwx+fl1L4t/Z1hurmS68LahHahzn7Hd7ii/7rjJA9iD9a4dvgD45WXYILFl/vi6GP5Z/SgCDxv8aPEHjGyk02OKLTdNk4khgYs8g9Gc9R7ADPfNbX7P3hCfUvFLeJJ4iLLTlZYmI4eZhjA9cKST6ErWt4a/ZwuTcJN4l1WFYQcm3scszexdgMfgD9a960vSrHRNMg07TbaO2tIF2xxIOAP6nuSeTQBcooooAKKK8y+Jvxdj8AahDpcGlNeX01uLhXeTZEilmUZxkk5Q8cduaAPTa5fxH8RPCvhUMuqaxAtwv/AC7RHzJc+m1ckfjgV8v+JPi14x8Tb47jVXtbZv8Al3sv3SY9CR8xH1JrA0Twrr3iWby9H0q6vDnBeNDsU+7n5R+JoA9h8SftH3Em+Hw1pCwr0FzfHc34IpwD9SfpWF8MvGXiLxT8Y9DfWdXubpc3BERbbGp8iTogwo/KvP8ATPDzy+OrPw3qW6GR9TSwufLYEoTKI2weQSOfUV9Y+Ffhb4U8H3Md5ptgz30YIW7uJC8gyCDj+EcEjgDrQB2VFFFABXzB+0d/yULT/wDsFR/+jZa+n6oaroul65am11XT7a9h/uTxh8e4z0PuKAPi/wAJ+IrHw5qX2m+8O6frKZHyXe75f93kr+amvozwx8c/BepRRWtx5miSABVjnj/dD2DLwB9QtZ3iT9njw/qG+bQryfS5jyIn/fRfqdw/M/SvIfEnwd8ZeG98j6ab+1X/AJb2JMox7rjcPxGKAEgnhuvj7HcW8qSwy+KA8ckbBldTdZBBHBBHevsOvge1urnTdQgu7Z2huraVZI3A5R1OQee4Ir3r4VfGLxL4h8Xaf4d1hbW6juRJ/pIj2SqVjZ/4flP3cdB1oA9+ooooAKKK5T4geOrLwD4f/tG5jNxcSv5dtbK20yPjPJ7KB1P09aAOror5Xn/aE8aSXJkiTTYY88RLbkjHuS2f1r1X4efGbT/FOn3o1tYdNvrCA3ExVj5UkQ+8y55BHGV5PIxnsAdh4i8B+GPFSt/a+kW80xH+vUbJR/wNcH8DxXneifC7w/4K+KuiXWn+JFMxM2zTLnDTMDDIMgr2HJ5A6dSa4vxv8fdY1aWWz8NKdMsclRcEAzyD19E/Dn3rn/gzNPd/GXRrieWSaVzcNJJIxZmPkSckmgD67ooooAK+a/2kr2R/Fej2BY+XDYmYD3d2B/8ARYr6Ur5g/aO/5KHp/wD2Co//AEbLQBd8F/AJfEHhez1jU9ZltXvIxNFDDCG2oeVJJPORg4x3rzrx14Qu/APiebR5brz0aISRTICnmxtnqM8cggjJ6V6v4d/aB0jRfDOlaVLol9JJZWcNuzrIgDFECkj24rzb4o+N7Xx94mttVtLSa1jis1tykrAkkO7Z47fOPyoA6b4b/BK48W2EOtazdNZaXKcxRxAGWZQcZyeFHocEn06Gvojw54Q0HwlafZ9F06G2BGHkAzJJ/vOeT/KvGfDnx/0bQ/DGlaU+iXrvZWkVuzo6AMyoFJH1IzXX+EPjfpfi/wAU2ehW2kXlvLdb9skjqVXajPzj2XFAHqVFFFABXiHxk+GXiXxn4vtNR0a3gkt47BIGMk6od4kkY8H2YV7fRQB8m/8AChfHf/Pnaf8AgUtH/ChfHf8Az52n/gUtfWVFAHyb/wAKF8d/8+dp/wCBS11nwz+Efi3wx8QtL1jU7a2Szt/N8xkuFYjdE6jge7CvoaigAooooAKKKKACiiigAooooAKKKKAP/9k="

// TestOCRRecognizeWithRotation_Live verifies layer-2 rotation selection
// end-to-end against a running DeepDoc rec service. It sends a vertical
// (tall, h/w >= 1.5) text crop at 0/CW90/CCW90 and asserts the orientation
// with the highest REAL recognition confidence reads the vertical text
// correctly — i.e. the Go path now does score-based selection (matching
// Python's get_rotate_crop_image) instead of the old constant-1.0 fallback.
//
// Uses the in-process NativeAnalyzer (real ONNX rec inference). Requires
// MODEL_DIR; the test is skipped when it is unset (ONNX Runtime is statically linked).
// Run with: build.sh --test-native ./internal/deepdoc/parser/pdf/
func TestOCRRecognizeWithRotation_Live(t *testing.T) {
	client := mustConnectInProcessAnalyzer(t)

	raw, err := base64.StdEncoding.DecodeString(layer2CropB64)
	if err != nil {
		t.Fatalf("decode embedded crop: %v", err)
	}
	crop, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode crop: %v", err)
	}

	cands := map[string]image.Image{
		"0":     crop,
		"CW90":  util.RotateImageCW(crop, 90),
		"CCW90": util.RotateImageCW(crop, 270),
	}
	scores := map[string]float64{}
	texts := map[string]string{}
	best, bestConf, bestText := "", -1.0, ""
	for name, im := range cands {
		rec, recErr := client.OCRRecognize(t.Context(), im)
		if recErr != nil {
			t.Fatalf("rec %s: %v", name, recErr)
		}
		c, txt := 0.0, ""
		if len(rec) > 0 {
			c, txt = rec[0].Confidence, rec[0].Text
		}
		scores[name], texts[name] = c, txt
		if c > bestConf {
			bestConf, best, bestText = c, name, txt
		}
	}

	// The rec service must surface REAL scores, not the old constant 1.0 fill.
	realScore := false
	for _, c := range scores {
		if c != 1.0 {
			realScore = true
		}
	}
	if !realScore {
		t.Fatalf("rec service returned constant 1.0; real score not surfaced: %v", scores)
	}

	// Layer 2 must find an upright orientation that reads the vertical text.
	if bestConf < 0.8 {
		t.Fatalf("best orientation confidence too low (%.3f); layer-2 failed: %v", bestConf, scores)
	}
	if !strings.Contains(strings.ToUpper(bestText), "RAG") {
		t.Fatalf("best orientation did not read the vertical text; got %q (scores=%v)", bestText, scores)
	}
	// 0° (as-is vertical) must score strictly worse than the best — proving
	// layer 2 actually does work rather than always trusting 0°.
	if scores["0"] >= bestConf {
		t.Fatalf("0° orientation scored >= best; layer-2 selection not effective: %v", scores)
	}
	t.Logf("layer-2 selected %s (conf=%.3f, text=%q); scores=%v", best, bestConf, bestText, scores)
}
