use sea_orm::entity::prelude::*;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, PartialEq, Eq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "kb_2_doc")]
pub struct Model {
    #[sea_orm(primary_key, auto_increment = false)]
    pub kb_id: i64,
    #[sea_orm(primary_key, auto_increment = false)]
    pub uid: i64,
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
                .from(Column::Uid)
                .to(super::doc_info::Column::Uid)
                .into(),
            Self::KbInfo => Entity::belongs_to(super::kb_info::Entity)
                .from(Column::KbId)
                .to(super::kb_info::Column::KbId)
                .into(),
        }
    }
}

impl ActiveModelBehavior for ActiveModel {}