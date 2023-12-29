use std::io::{Cursor, Write};
use std::time::{Duration, Instant};
use actix_rt::time::interval;
use actix_web::{HttpRequest, HttpResponse, rt, web};
use actix_web::web::Buf;
use actix_ws::Message;
use futures_util::{future, StreamExt};
use futures_util::future::Either;
use uuid::Uuid;
use crate::api::doc_info::_upload_file;
use crate::AppState;
use crate::errors::AppError;

const HEARTBEAT_INTERVAL: Duration = Duration::from_secs(5);

/// How long before lack of client response causes a timeout.
const CLIENT_TIMEOUT: Duration = Duration::from_secs(10);

pub async fn upload_file_ws(req: HttpRequest, stream: web::Payload, data: web::Data<AppState>) -> Result<HttpResponse, AppError> {
    let (res, session, msg_stream) = actix_ws::handle(&req, stream)?;

    // spawn websocket handler (and don't await it) so that the response is returned immediately
    rt::spawn(upload_file_handler(data, session, msg_stream));

    Ok(res)
}

async fn upload_file_handler(
    data: web::Data<AppState>,
    mut session: actix_ws::Session,
    mut msg_stream: actix_ws::MessageStream,
) {
    let mut bytes = Cursor::new(vec![]);
    let mut last_heartbeat = Instant::now();
    let mut interval = interval(HEARTBEAT_INTERVAL);

    let reason = loop {
        let tick = interval.tick();
        tokio::pin!(tick);

        match future::select(msg_stream.next(), tick).await {
            // received message from WebSocket client
            Either::Left((Some(Ok(msg)), _)) => {
                match msg {
                    Message::Text(text) => {
                        session.text(text).await.unwrap();
                    }

                    Message::Binary(bin) => {
                        let mut pos = 0;  // notice the name of the file that will be written
                        while pos < bin.len() {
                            let bytes_written = bytes.write(&bin[pos..]).unwrap();
                            pos += bytes_written
                        };
                        session.binary(bin).await.unwrap();
                    }

                    Message::Close(reason) => {
                        break reason;
                    }

                    Message::Ping(bytes) => {
                        last_heartbeat = Instant::now();
                        let _ = session.pong(&bytes).await;
                    }

                    Message::Pong(_) => {
                        last_heartbeat = Instant::now();
                    }

                    Message::Continuation(_) | Message::Nop => {}
                };
            }
            Either::Left((Some(Err(_)), _)) => {
                break None;
            }
            Either::Left((None, _)) => break None,
            Either::Right((_inst, _)) => {
                if Instant::now().duration_since(last_heartbeat) > CLIENT_TIMEOUT {
                    break None;
                }

                let _ = session.ping(b"").await;
            }
        }
    };
    let _ = session.close(reason).await;

    if !bytes.has_remaining() {
        return;
    }

    let uid = bytes.get_i64();
    let did = bytes.get_i64();

    _upload_file(uid, did, &Uuid::new_v4().to_string(), &bytes.into_inner(), &data).await.unwrap();
}