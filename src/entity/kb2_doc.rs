use sea_orm::entity::prelude::*;
use serde::{Deserialize, Serialize};
use chrono::{DateTime, FixedOffset};

#[derive(Clone, Debug, PartialEq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "kb2_doc")]
pub struct Model {
    #[sea_orm(primary_key, auto_increment = true)]
    pub id: i64,
    #[sea_orm(index)]
    pub kb_id: i64,
    #[sea_orm(index)]
    pub did: i64,
<<<<<<< HEAD
    #[serde(skip_deserializing)]
    pub kb_progress: f32,
    #[serde(skip_deserializing)]
    pub kb_progress_msg: String,
    #[serde(skip_deserializing)]
    pub updated_at: DateTime<FixedOffset>,
    #[serde(skip_deserializing)]
    pub is_deleted: bool,
=======
>>>>>>> upstream/main
}

#[derive(Debug, Clone, Copy, EnumIter)]
pub enum Relation {
    DocInfo,
    KbInfo,
}

impl RelationTrait for Relation {
    fn def(&self) -> RelationDef {
        match self {
            Self::DocInfo => Entity::belongs_to(super::doc_info::Entity)
                .from(Column::Did)
                .to(super::doc_info::Column::Did)
                .into(),
            Self::KbInfo => Entity::belongs_to(super::kb_info::Entity)
                .from(Column::KbId)
                .to(super::kb_info::Column::KbId)
                .into(),
        }
    }
}

impl ActiveModelBehavior for ActiveModel {}