use sea_orm::entity::prelude::*;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, PartialEq, Eq, DeriveEntityModel, Deserialize, Serialize)]
#[sea_orm(table_name = "dialog2_kb")]
pub struct Model {
    #[sea_orm(primary_key, auto_increment = false)]
    #[sea_orm(index)]
    pub dialog_id: i64,
    #[sea_orm(primary_key, auto_increment = false)]
    pub kb_id: i64,
}

#[derive(Debug, Clone, Copy, EnumIter)]
pub enum Relation {
    DialogInfo,
    KbInfo,
}

impl RelationTrait for Relation {
    fn def(&self) -> RelationDef {
        match self {
            Self::DialogInfo => Entity::belongs_to(super::dialog_info::Entity)
                .from(Column::DialogId)
                .to(super::dialog_info::Column::DialogId)
                .into(),
            Self::KbInfo => Entity::belongs_to(super::kb_info::Entity)
                .from(Column::KbId)
                .to(super::kb_info::Column::KbId)
                .into(),
        }
    }
}

impl ActiveModelBehavior for ActiveModel {}