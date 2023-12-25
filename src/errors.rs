use actix_web::{ HttpResponse, ResponseError };
use thiserror::Error;

#[derive(Debug, Error)]
pub(crate) enum AppError {
    #[error("`{0}`")] User(#[from] UserError),

    #[error("`{0}`")] Json(#[from] serde_json::Error),

    #[error("`{0}`")] Actix(#[from] actix_web::Error),

    #[error("`{0}`")] Db(#[from] sea_orm::DbErr),

    #[error("`{0}`")] Std(#[from] std::io::Error),
}

#[derive(Debug, Error)]
pub(crate) enum UserError {
    #[error("`username` field of `User` cannot be empty!")]
    EmptyUsername,

    #[error("`username` field of `User` cannot contain whitespaces!")]
    UsernameInvalidCharacter,

    #[error("`password` field of `User` cannot be empty!")]
    EmptyPassword,

    #[error("`password` field of `User` cannot contain whitespaces!")]
    PasswordInvalidCharacter,

    #[error("Could not find any `User` for id: `{0}`!")] NotFound(i64),

    #[error("Failed to login user!")]
    LoginFailed,

    #[error("User is not logged in!")]
    NotLoggedIn,

    #[error("Invalid authorization token!")]
    InvalidToken,

    #[error("Could not find any `User`!")]
    Empty,
}

impl ResponseError for AppError {
    fn status_code(&self) -> actix_web::http::StatusCode {
        match self {
            AppError::User(user_error) =>
                match user_error {
                    UserError::EmptyUsername => actix_web::http::StatusCode::UNPROCESSABLE_ENTITY,
                    UserError::UsernameInvalidCharacter => {
                        actix_web::http::StatusCode::UNPROCESSABLE_ENTITY
                    }
                    UserError::EmptyPassword => actix_web::http::StatusCode::UNPROCESSABLE_ENTITY,
                    UserError::PasswordInvalidCharacter => {
                        actix_web::http::StatusCode::UNPROCESSABLE_ENTITY
                    }
                    UserError::NotFound(_) => actix_web::http::StatusCode::NOT_FOUND,
                    UserError::NotLoggedIn => actix_web::http::StatusCode::UNAUTHORIZED,
                    UserError::Empty => actix_web::http::StatusCode::NOT_FOUND,
                    UserError::LoginFailed => actix_web::http::StatusCode::NOT_FOUND,
                    UserError::InvalidToken => actix_web::http::StatusCode::UNAUTHORIZED,
                }
            AppError::Json(_) => actix_web::http::StatusCode::INTERNAL_SERVER_ERROR,
            AppError::Actix(fail) => fail.as_response_error().status_code(),
            AppError::Db(_) => actix_web::http::StatusCode::INTERNAL_SERVER_ERROR,
            AppError::Std(_) => actix_web::http::StatusCode::INTERNAL_SERVER_ERROR,
        }
    }

    fn error_response(&self) -> HttpResponse {
        let status_code = self.status_code();
        let response = HttpResponse::build(status_code).body(self.to_string());
        response
    }
}
