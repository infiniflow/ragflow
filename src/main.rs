mod api;
mod entity;
mod service;
mod errors;

use std::env;
use actix_files::Files;
use actix_identity::{CookieIdentityPolicy, IdentityService, RequestIdentity};
use actix_session::CookieSession;
use actix_web::{web, App, HttpServer, middleware, Error};
use actix_web::cookie::time::Duration;
use actix_web::dev::ServiceRequest;
use actix_web::error::ErrorUnauthorized;
use actix_web_httpauth::extractors::bearer::BearerAuth;
use listenfd::ListenFd;
use sea_orm::{Database, DatabaseConnection};
use migration::{Migrator, MigratorTrait};
use crate::errors::UserError;

#[derive(Debug, Clone)]
struct AppState {
    conn: DatabaseConnection,
}

pub(crate) async fn validator(
    req: ServiceRequest,
    credentials: BearerAuth,
) -> Result<ServiceRequest, Error> {
    if let Some(token) = req.get_identity() {
        println!("{}, {}",credentials.token(), token);
        (credentials.token() == token)
            .then(|| req)
            .ok_or(ErrorUnauthorized(UserError::InvalidToken))
    } else {
        Err(ErrorUnauthorized(UserError::NotLoggedIn))
    }
}

#[actix_web::main]
async fn main() -> std::io::Result<()> {
    std::env::set_var("RUST_LOG", "debug");
    tracing_subscriber::fmt::init();

    // get env vars
    dotenvy::dotenv().ok();
    let db_url = env::var("DATABASE_URL").expect("DATABASE_URL is not set in .env file");
    let host = env::var("HOST").expect("HOST is not set in .env file");
    let port = env::var("PORT").expect("PORT is not set in .env file");
    let server_url = format!("{host}:{port}");

    // establish connection to database and apply migrations
    // -> create post table if not exists
    let conn = Database::connect(&db_url).await.unwrap();
    Migrator::up(&conn, None).await.unwrap();

    let state = AppState { conn };

    // create server and try to serve over socket if possible
    let mut listenfd = ListenFd::from_env();
    let mut server = HttpServer::new(move || {
        App::new()
            .service(Files::new("/static", "./static"))
            .app_data(web::Data::new(state.clone()))
            .wrap(IdentityService::new(
                CookieIdentityPolicy::new(&[0; 32])
                    .name("auth-cookie")
                    .login_deadline(Duration::seconds(120))
                    .secure(false),
            ))
            .wrap(
                CookieSession::signed(&[0; 32])
                    .name("session-cookie")
                    .secure(false)
                    // WARNING(alex): This uses the `time` crate, not `std::time`!
                    .expires_in_time(Duration::seconds(60)),
            )
            .wrap(middleware::Logger::default())
            .configure(init)
    });

    server = match listenfd.take_tcp_listener(0)? {
        Some(listener) => server.listen(listener)?,
        None => server.bind(&server_url)?,
    };

    println!("Starting server at {server_url}");
    server.run().await?;

    Ok(())
}

fn init(cfg: &mut web::ServiceConfig) {
    cfg.service(api::tag_info::create);
    cfg.service(api::tag_info::delete);
    cfg.service(api::tag_info::list);

    cfg.service(api::kb_info::create);
    cfg.service(api::kb_info::delete);
    cfg.service(api::kb_info::list);
    cfg.service(api::kb_info::add_docs_to_kb);
<<<<<<< HEAD
    cfg.service(api::kb_info::anti_kb_docs);
    cfg.service(api::kb_info::all_relevents);
=======
>>>>>>> upstream/main

    cfg.service(api::doc_info::list);
    cfg.service(api::doc_info::delete);
    cfg.service(api::doc_info::mv);
    cfg.service(api::doc_info::upload);
    cfg.service(api::doc_info::new_folder);
    cfg.service(api::doc_info::rename);

    cfg.service(api::dialog_info::list);
    cfg.service(api::dialog_info::delete);
    cfg.service(api::dialog_info::create);
    cfg.service(api::dialog_info::update_history);

    cfg.service(api::user_info::login);
    cfg.service(api::user_info::register);
    cfg.service(api::user_info::setting);
}