using System;
using System.Collections;
using System.Collections.Generic;
using Data;
using TMPro;
using UnityEngine;
using UnityEngine.UI;

public class CardRow : MonoBehaviour
{
    [SerializeField] private Image _iconImage;
    [SerializeField] private TextMeshProUGUI _nameText;
    [SerializeField] private TextMeshProUGUI _isuRateText;
    [SerializeField] private TextMeshProUGUI _levelText;
    [SerializeField] private TextMeshProUGUI _pointText;
    [SerializeField] private Button _enhanceButton;
    [SerializeField] private Button _equipButton;
    [SerializeField] private Toggle _equipToggle;

    public bool IsEquipOn => _equipToggle.isOn;

    private UserCard _card;

    private Action<UserCard> _onEnhanceButtonPressed;
    private Action<UserCard> _onEquipToggleChanged;
    
    private void Start()
    {
        _enhanceButton.onClick.AddListener(() => Enhance());
        _equipButton.onClick.AddListener(() => Equip());
        _equipToggle.onValueChanged.AddListener((_) => Equip());
    }
    
    public void SetCard(UserCard card, Action<UserCard> onEnhanceButtonPressed, Action<UserCard> onEquipToggleChanged)
    {
        _card = card;
        _onEnhanceButtonPressed = onEnhanceButtonPressed;
        _onEquipToggleChanged = onEquipToggleChanged;
        
        var master = StaticItemMaster.Items[card.cardId];
        _nameText.text = master.name;
        _isuRateText.text = $"{card.amountPerSec} ISU/s";
        _levelText.text = $"{card.level}";
        _pointText.text = $"{card.totalExp}";
        _equipToggle.isOn = false;
    }

    private async void Enhance()
    {
        _onEnhanceButtonPressed?.Invoke(_card);
    }

    private async void Equip()
    {
        _onEquipToggleChanged?.Invoke(_card);
    }
}
