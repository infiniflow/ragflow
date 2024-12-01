<?php

$curl = curl_init();

curl_setopt_array($curl, array(
  CURLOPT_URL => 'http://127.0.0.1/api/v1/datasets/8a85ab34ad5311ef98b00242ac120003/documents/501e387aadf411ef922e0242ac120003/chunks?keywords=%E6%B9%BF%E6%B6%A6%E5%8D%8A%E5%B9%B2%E6%97%B1',
  CURLOPT_RETURNTRANSFER => true,
  CURLOPT_ENCODING => '',
  CURLOPT_MAXREDIRS => 10,
  CURLOPT_TIMEOUT => 0,
  CURLOPT_FOLLOWLOCATION => true,
  CURLOPT_HTTP_VERSION => CURL_HTTP_VERSION_1_1,
  CURLOPT_CUSTOMREQUEST => 'GET',
  CURLOPT_HTTPHEADER => array(
    'Authorization: Bearer ragflow-hjNzA4ODI4YWM5MTExZWY5YzUyMDI0Mm'
  ),
));

$response = curl_exec($curl);

curl_close($curl);
echo $response;
