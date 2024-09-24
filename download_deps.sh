#!/usr/bin/env bash

download()
{
    echo "download $1"
    # https://stackoverflow.com/questions/3162385/how-to-split-a-string-in-shell-and-get-the-last-field
    fn=${1##*/}
    if [ ! -f $fn ] ; then
        wget --no-check-certificate $1
    fi
}

# https://stackoverflow.com/questions/24628076/convert-multiline-string-to-array
names="https://huggingface.co/InfiniFlow/deepdoc/resolve/main/det.onnx
https://huggingface.co/InfiniFlow/deepdoc/resolve/main/layout.laws.onnx
https://huggingface.co/InfiniFlow/deepdoc/resolve/main/layout.manual.onnx
https://huggingface.co/InfiniFlow/deepdoc/resolve/main/layout.onnx
https://huggingface.co/InfiniFlow/deepdoc/resolve/main/layout.paper.onnx
https://huggingface.co/InfiniFlow/deepdoc/resolve/main/ocr.res
https://huggingface.co/InfiniFlow/deepdoc/resolve/main/rec.onnx
https://huggingface.co/InfiniFlow/deepdoc/resolve/main/tsr.onnx
https://huggingface.co/InfiniFlow/text_concat_xgb_v1.0/resolve/main/updown_concat_xgb.model"

SAVEIFS=$IFS   # Save current IFS (Internal Field Separator)
IFS=$'\n'      # Change IFS to newline char
names=($names) # split the `names` string into an array by the same name
IFS=$SAVEIFS   # Restore original IFS

find . -size 0 | xargs rm -f
# https://stackoverflow.com/questions/15466808/shell-iterate-over-array
for ((i=0; i<${#names[@]}; i+=1)); do
    url="${names[$i]}"
    download $url
    if [ $? != 0 ]; then
	exit -1
    fi
done
find . -size 0 | xargs rm -f
